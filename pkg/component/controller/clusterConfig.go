/*
Copyright 2021 k0s authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	k0sclient "github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/clientset/typed/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/component/controller/clusterconfig"
	"github.com/k0sproject/k0s/pkg/component/controller/leaderelector"
	"github.com/k0sproject/k0s/pkg/component/manager"
	"github.com/k0sproject/k0s/pkg/constant"
	kubeutil "github.com/k0sproject/k0s/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/sirupsen/logrus"
)

// ClusterConfigReconciler reconciles a ClusterConfig object
type ClusterConfigReconciler struct {
	ComponentManager  *manager.Manager
	KubeClientFactory kubeutil.ClientFactoryInterface

	configClient  k0sclient.ClusterConfigInterface
	leaderElector leaderelector.Interface
	log           *logrus.Entry
	configSource  clusterconfig.ConfigSource
	initialConfig *v1beta1.ClusterConfig
}

// NewClusterConfigReconciler creates a new clusterConfig reconciler
func NewClusterConfigReconciler(leaderElector leaderelector.Interface, k0sVars constant.CfgVars, mgr *manager.Manager, kubeClientFactory kubeutil.ClientFactoryInterface, configSource clusterconfig.ConfigSource, initialConfig *v1beta1.ClusterConfig) (*ClusterConfigReconciler, error) {
	configClient, err := kubeClientFactory.GetConfigClient()
	if err != nil {
		return nil, err
	}

	return &ClusterConfigReconciler{
		ComponentManager:  mgr,
		KubeClientFactory: kubeClientFactory,
		leaderElector:     leaderElector,
		log:               logrus.WithFields(logrus.Fields{"component": "clusterConfig-reconciler"}),
		configSource:      configSource,
		configClient:      configClient,
		initialConfig:     initialConfig,
	}, nil
}

func (r *ClusterConfigReconciler) Init(context.Context) error { return nil }

func (r *ClusterConfigReconciler) Start(ctx context.Context) error {
	if r.initialConfig != nil {
		// We need to wait until the cluster configuration exists or we succeed in creating it.
		err := wait.PollImmediateWithContext(ctx, 1*time.Second, 20*time.Second, func(ctx context.Context) (bool, error) {
			var err error

			if r.leaderElector.IsLeader() {
				err = r.createClusterConfig(ctx)
				if err == nil {
					r.log.Debug("Cluster configuration created")
					return true, nil
				}
				if errors.IsAlreadyExists(err) {
					// An already existing configuration is just fine.
					r.log.Debug("Cluster configuration already exists")
					return true, nil
				}
				return false, fmt.Errorf("failed to create cluster config: %w", err)
			}

			err = r.clusterConfigExists(ctx)
			if err == nil {
				r.log.Debug("Cluster configuration exists")
				return true, nil
			}
			r.log.WithError(err).Debug("Failed to ensure the existence of the cluster configuration")
			return false, err
		})
		if err != nil {
			return fmt.Errorf("failed to ensure the existence of the cluster configuration: %w", err)
		}
	}

	go func() {
		statusCtx := ctx
		ch := r.configSource.ResultChan()
		r.log.Debug("start listening changes from config source")
		for {
			select {
			case cfg, ok := <-ch:
				if !ok {
					// Recv channel close, we can stop now
					r.log.Debug("config source closed channel")
					return
				}
				err := cfg.ValidationError()
				if err == nil {
					err = r.ComponentManager.Reconcile(ctx, cfg)
				}
				r.reportStatus(statusCtx, cfg, err)
				if err != nil {
					r.log.WithError(err).Error("Failed to reconcile cluster configuration")
				} else {
					r.log.Debug("Successfully reconciled cluster configuration")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop stops
func (r *ClusterConfigReconciler) Stop() error {
	// Nothing really to stop, the main ConfigSource "watch" channel go-routine is stopped
	// via the main Context's Done channel in the Run function
	return nil
}

func (r *ClusterConfigReconciler) reportStatus(ctx context.Context, config *v1beta1.ClusterConfig, reconcileError error) {
	hostname, err := os.Hostname()
	if err != nil {
		r.log.Error("failed to get hostname:", err)
		hostname = ""
	}
	// TODO We need to design proper status field(s) to the cluster cfg object, now just send event
	client, err := r.KubeClientFactory.GetClient()
	if err != nil {
		r.log.Error("failed to get kube client:", err)
	}
	e := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "k0s.",
		},
		EventTime:      metav1.NowMicro(),
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
		InvolvedObject: corev1.ObjectReference{
			Kind:            v1beta1.ClusterConfigKind,
			Namespace:       config.Namespace,
			Name:            config.Name,
			UID:             config.UID,
			APIVersion:      v1beta1.ClusterConfigAPIVersion,
			ResourceVersion: config.ResourceVersion,
		},
		Action:              "ConfigReconciling",
		ReportingController: "k0s-controller",
		ReportingInstance:   hostname,
	}
	if reconcileError != nil {
		e.Reason = "FailedReconciling"
		e.Message = reconcileError.Error()
		e.Type = corev1.EventTypeWarning
	} else {
		e.Reason = "SuccessfulReconcile"
		e.Message = "Successfully reconciled cluster config"
		e.Type = corev1.EventTypeNormal
	}
	_, err = client.CoreV1().Events(constant.ClusterConfigNamespace).Create(ctx, e, metav1.CreateOptions{})
	if err != nil {
		r.log.Error("failed to create event for config reconcile:", err)
	}
}

func (r *ClusterConfigReconciler) clusterConfigExists(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.configClient.Get(ctx, constant.ClusterConfigObjectName,
		metav1.GetOptions{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.ClusterConfigAPIVersion,
				Kind:       "clusterconfigs",
			},
		},
	)
	return err
}

func (r *ClusterConfigReconciler) createClusterConfig(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.configClient.Create(ctx, r.initialConfig, metav1.CreateOptions{})
	return err
}
