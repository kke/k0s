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

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	cfgClient "github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/clientset"
	k0sv1beta1 "github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/clientset/typed/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/constant"
)

var (
	resourceType = v1.TypeMeta{APIVersion: "k0s.k0sproject.io/v1beta1", Kind: "clusterconfigs"}
	getOpts      = v1.GetOptions{TypeMeta: resourceType}
)

func FromAPI(client k0sv1beta1.K0sV1beta1Interface) (*v1beta1.ClusterConfig, error) {
	var cfg *v1beta1.ClusterConfig
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	// clear up context after timeout
	defer cancel()

	err = retry.Do(func() error {
		logrus.Debug("fetching cluster-config from API...")
		cfg, err = configRequest(client)
		if err != nil {
			return err
		}
		return nil
	}, retry.Context(ctx))

	if err != nil {
		return nil, fmt.Errorf("timed out waiting for API to return cluster-config: %w", err)
	}

	return cfg, nil
}

func configRequest(client k0sv1beta1.K0sV1beta1Interface) (clusterConfig *v1beta1.ClusterConfig, err error) {
	clusterConfigs := client.ClusterConfigs(constant.ClusterConfigNamespace)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	defer cancel()

	cfg, err := clusterConfigs.Get(ctx, "k0s", getOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cluster-config from API: %v", err)
	}
	return cfg, nil
}

func apiClient(vars constant.CfgVars) (k0sv1beta1.K0sV1beta1Interface, error) {
	// generate a kubernetes client from AdminKubeConfigPath
	config, err := clientcmd.BuildConfigFromFlags("", vars.AdminKubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("can't read kubeconfig: %w", err)
	}
	client, err := cfgClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("can't create kubernetes typed client for cluster config: %w", err)
	}

	return client.K0sV1beta1(), nil
}
