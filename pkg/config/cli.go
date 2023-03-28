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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	aproot "github.com/k0sproject/k0s/pkg/autopilot/controller/root"
	"github.com/k0sproject/k0s/pkg/component/manager"
	"github.com/k0sproject/k0s/pkg/constant"

	cloudprovider "k8s.io/cloud-provider"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
)

// This struct holds all the CLI options & settings required by the
// different k0s sub-commands
type CLIOptions struct {
	WorkerOptions
	ControllerOptions
	CfgFile          string
	Debug            bool
	DebugListenOn    string
	DefaultLogLevels map[string]string
	Logging          map[string]string // merged outcome of default log levels and cmdLoglevels
	Verbose          bool
	AutopilotRoot    aproot.Root
	DataDir          string
	StatusSocket     string
	nodeConfig       *v1beta1.ClusterConfig
	k0sVars          *constant.CfgVars
}

func DefaultCLIOptions() *CLIOptions {
	return &CLIOptions{
		DefaultLogLevels: DefaultLogLevels(),
		WorkerOptions: WorkerOptions{
			CmdLogLevels: DefaultLogLevels(),
		},
	}
}

func (o *CLIOptions) K0sVars() *constant.CfgVars {
	if o.k0sVars == nil {
		o.k0sVars = constant.GetConfig(o.DataDir)
	}
	return o.k0sVars
}

func (o *CLIOptions) DefaultStorageType() string {
	if o.ControllerOptions.SingleNode {
		return "kine"
	}
	return "etcd"
}

func (o *CLIOptions) NodeConfig() *v1beta1.ClusterConfig {
	if o.nodeConfig == nil {
		o.nodeConfig = getNodeConfig(o)
	}
	return o.nodeConfig
}

// Shared controller cli flags
type ControllerOptions struct {
	EnableWorker      bool
	SingleNode        bool
	NoTaints          bool
	DisableComponents []string

	ClusterComponents               *manager.Manager
	EnableK0sCloudProvider          bool
	K0sCloudProviderPort            int
	K0sCloudProviderUpdateFrequency time.Duration
	NodeComponents                  *manager.Manager
	EnableDynamicConfig             bool
	EnableMetricsScraper            bool
	KubeControllerManagerExtraArgs  string
}

// Shared worker cli flags
type WorkerOptions struct {
	APIServer        string
	CIDRRange        string
	CloudProvider    bool
	ClusterDNS       string
	CmdLogLevels     map[string]string
	CriSocket        string
	KubeletExtraArgs string
	Labels           []string
	Taints           []string
	TokenFile        string
	TokenArg         string
	WorkerProfile    string
	IPTablesMode     string
}

func (o *ControllerOptions) Normalize() error {
	if o.SingleNode {
		o.EnableWorker = true
	}

	// Normalize component names
	var disabledComponents []string
	for _, disabledComponent := range o.DisableComponents {
		if disabledComponent == constant.KubeletConfigComponentName {
			logrus.Warnf("Usage of deprecated component name %q, please switch to %q",
				constant.KubeletConfigComponentName, constant.WorkerConfigComponentName,
			)
			disabledComponent = constant.WorkerConfigComponentName
		}

		if !slices.Contains(availableComponents, disabledComponent) {
			return fmt.Errorf("unknown component %s", disabledComponent)
		}

		if !slices.Contains(disabledComponents, disabledComponent) {
			disabledComponents = append(disabledComponents, disabledComponent)
		}
	}
	o.DisableComponents = disabledComponents

	return nil
}

func DefaultLogLevels() map[string]string {
	return map[string]string{
		"etcd":                    "info",
		"containerd":              "info",
		"konnectivity-server":     "1",
		"kube-apiserver":          "1",
		"kube-controller-manager": "1",
		"kube-scheduler":          "1",
		"kubelet":                 "1",
		"kube-proxy":              "1",
	}
}

func GetPersistentFlagSet(opts *CLIOptions) *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	flagset.BoolVarP(&opts.Debug, "debug", "d", false, "Debug logging (default: false)")
	flagset.BoolVarP(&opts.Verbose, "verbose", "v", false, "Verbose logging (default: false)")
	flagset.StringVar(&opts.DataDir, "data-dir", "", "Data Directory for k0s (default: /var/lib/k0s). DO NOT CHANGE for an existing setup, things will break!")
	flagset.StringVar(&opts.StatusSocket, "status-socket", filepath.Join(opts.K0sVars().RunDir, "status.sock"), "Full file path to the socket file.")
	flagset.StringVar(&opts.DebugListenOn, "debugListenOn", ":6060", "Http listenOn for Debug pprof handler")
	return flagset
}

// XX: not a pretty hack, but we need the data-dir flag for the kubectl subcommand
// XX: when other global flags cannot be used (specifically -d and -c)
func GetKubeCtlFlagSet(opts *CLIOptions) *pflag.FlagSet {
	debugDefault := false
	if v, ok := os.LookupEnv("DEBUG"); ok {
		debugDefault, _ = strconv.ParseBool(v)
	}

	flagset := &pflag.FlagSet{}
	flagset.StringVar(&opts.DataDir, "data-dir", "", "Data Directory for k0s (default: /var/lib/k0s). DO NOT CHANGE for an existing setup, things will break!")
	flagset.BoolVar(&opts.Debug, "debug", debugDefault, "Debug logging [$DEBUG]")
	return flagset
}

func GetCriSocketFlag(opts *CLIOptions) *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	workerOpts := &opts.WorkerOptions
	flagset.StringVar(&workerOpts.CriSocket, "cri-socket", "", "container runtime socket to use, default to internal containerd. Format: [remote|docker]:[path-to-socket]")
	return flagset
}

func GetWorkerFlags(opts *CLIOptions) *pflag.FlagSet {
	flagset := &pflag.FlagSet{}

	workerOpts := &opts.WorkerOptions
	flagset.StringVar(&workerOpts.WorkerProfile, "profile", "default", "worker profile to use on the node")
	flagset.StringVar(&workerOpts.APIServer, "api-server", "", "HACK: api-server for the windows worker node")
	flagset.StringVar(&workerOpts.CIDRRange, "cidr-range", "10.96.0.0/12", "HACK: cidr range for the windows worker node")
	flagset.StringVar(&workerOpts.ClusterDNS, "cluster-dns", "10.96.0.10", "HACK: cluster dns for the windows worker node")
	flagset.BoolVar(&workerOpts.CloudProvider, "enable-cloud-provider", false, "Whether or not to enable cloud provider support in kubelet")
	flagset.StringVar(&workerOpts.TokenFile, "token-file", "", "Path to the file containing token.")
	flagset.StringToStringVarP(&workerOpts.CmdLogLevels, "logging", "l", DefaultLogLevels(), "Logging Levels for the different components")
	flagset.StringSliceVarP(&workerOpts.Labels, "labels", "", []string{}, "Node labels, list of key=value pairs")
	flagset.StringSliceVarP(&workerOpts.Taints, "taints", "", []string{}, "Node taints, list of key=value:effect strings")
	flagset.StringVar(&workerOpts.KubeletExtraArgs, "kubelet-extra-args", "", "extra args for kubelet")
	flagset.StringVar(&workerOpts.IPTablesMode, "iptables-mode", "", "iptables mode (valid values: nft, legacy, auto). default: auto")
	flagset.AddFlagSet(GetCriSocketFlag(opts))

	return flagset
}

var availableComponents = []string{
	constant.APIConfigComponentName,
	constant.AutopilotComponentName,
	constant.ControlAPIComponentName,
	constant.CoreDNSComponentname,
	constant.CsrApproverComponentName,
	constant.APIEndpointReconcilerComponentName,
	constant.HelmComponentName,
	constant.KonnectivityServerComponentName,
	constant.KubeControllerManagerComponentName,
	constant.KubeProxyComponentName,
	constant.KubeSchedulerComponentName,
	constant.MetricsServerComponentName,
	constant.NetworkProviderComponentName,
	constant.NodeRoleComponentName,
	constant.SystemRbacComponentName,
	constant.WorkerConfigComponentName,
}

func GetControllerFlags(opts *CLIOptions) *pflag.FlagSet {
	flagset := &pflag.FlagSet{}

	workerOpts := &opts.WorkerOptions
	controllerOpts := &opts.ControllerOptions

	flagset.StringVar(&workerOpts.WorkerProfile, "profile", "default", "worker profile to use on the node")
	flagset.BoolVar(&controllerOpts.EnableWorker, "enable-worker", false, "enable worker (default false)")
	flagset.StringSliceVar(&controllerOpts.DisableComponents, "disable-components", []string{}, "disable components (valid items: "+strings.Join(availableComponents, ",")+")")
	flagset.StringVar(&workerOpts.TokenFile, "token-file", "", "Path to the file containing join-token.")
	flagset.StringToStringVarP(&workerOpts.CmdLogLevels, "logging", "l", DefaultLogLevels(), "Logging Levels for the different components")
	flagset.BoolVar(&controllerOpts.SingleNode, "single", false, "enable single node (implies --enable-worker, default false)")
	flagset.BoolVar(&controllerOpts.NoTaints, "no-taints", false, "disable default taints for controller node")
	flagset.BoolVar(&controllerOpts.EnableK0sCloudProvider, "enable-k0s-cloud-provider", false, "enables the k0s-cloud-provider (default false)")
	flagset.DurationVar(&controllerOpts.K0sCloudProviderUpdateFrequency, "k0s-cloud-provider-update-frequency", 2*time.Minute, "the frequency of k0s-cloud-provider node updates")
	flagset.IntVar(&controllerOpts.K0sCloudProviderPort, "k0s-cloud-provider-port", cloudprovider.CloudControllerManagerPort, "the port that k0s-cloud-provider binds on")
	flagset.AddFlagSet(GetCriSocketFlag(opts))
	flagset.BoolVar(&controllerOpts.EnableDynamicConfig, "enable-dynamic-config", false, "enable cluster-wide dynamic config based on custom resource")
	flagset.BoolVar(&controllerOpts.EnableMetricsScraper, "enable-metrics-scraper", false, "enable scraping metrics from the controller components (kube-scheduler, kube-controller-manager)")
	flagset.StringVar(&controllerOpts.KubeControllerManagerExtraArgs, "kube-controller-manager-extra-args", "", "extra args for kube-controller-manager")
	flagset.AddFlagSet(FileInputFlag(opts))
	return flagset
}

// The config flag used to be a persistent, joint flag to all commands
// now only a few commands use it. This function helps to share the flag with multiple commands without needing to define
// it in multiple places
func FileInputFlag(opts *CLIOptions) *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	descString := fmt.Sprintf("config file, use '-' to read the config from stdin (default \"%s\")", constant.K0sConfigPathDefault)
	flagset.StringVarP(&opts.CfgFile, "config", "c", "", descString)

	return flagset
}

// CallParentPersistentPreRun runs the parent command's persistent pre-run.
// Cobra does not do this automatically.
//
// See: https://github.com/spf13/cobra/issues/216
// See: https://github.com/spf13/cobra/blob/v1.4.0/command.go#L833-L843
func CallParentPersistentPreRun(c *cobra.Command, args []string) error {
	for p := c.Parent(); p != nil; p = p.Parent() {
		preRunE := p.PersistentPreRunE
		preRun := p.PersistentPreRun

		p.PersistentPreRunE = nil
		p.PersistentPreRun = nil

		defer func() {
			p.PersistentPreRunE = preRunE
			p.PersistentPreRun = preRun
		}()

		if preRunE != nil {
			return preRunE(c, args)
		}

		if preRun != nil {
			preRun(c, args)
			return nil
		}
	}

	return nil
}

func PreRunValidateConfig(opts *CLIOptions) error {
	loadingRules := ClientConfigLoadingRules{Opts: opts}
	_, err := loadingRules.ParseRuntimeConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %v", err)
	}
	return nil
}

func getNodeConfig(opts *CLIOptions) *v1beta1.ClusterConfig {
	loadingRules := ClientConfigLoadingRules{Nodeconfig: true, Opts: opts}
	cfg, err := loadingRules.Load()
	if err != nil {
		return nil
	}
	return cfg
}

func LoadClusterConfig(opts *CLIOptions) (*v1beta1.ClusterConfig, error) {
	loadingRules := ClientConfigLoadingRules{Opts: opts}
	return loadingRules.Load()
}
