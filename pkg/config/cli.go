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
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	aproot "github.com/k0sproject/k0s/pkg/autopilot/controller/root"
	"github.com/k0sproject/k0s/pkg/constant"
	"github.com/k0sproject/k0s/pkg/k0scloudprovider"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
)

var (
	CfgFile       string
	DataDir       string
	Debug         bool
	DebugListenOn string
	StatusSocket  string
	Verbose       bool

	workerOpts     WorkerOptions
	controllerOpts ControllerOptions
)

// This struct holds all the CLI options & settings required by the
// different k0s sub-commands
type CLIOptions struct {
	WorkerOptions
	ControllerOptions

	CfgFile       string
	DataDir       string
	Debug         bool
	DebugListenOn string
	StatusSocket  string
	Verbose       bool

	K0sVars constant.CfgVars

	AutopilotRoot aproot.Root

	initialConfig   *v1beta1.ClusterConfig
	bootstrapConfig *v1beta1.ClusterConfig

	stdin io.Reader
}

// Copy returns a deep copy of the CLIOptions struct
// todo: this is a bit cumbersome if the fields in any of the structs change,
// it's easy to forget to update this method.
func (o *CLIOptions) Copy() CLIOptions {
	var initialConfig *v1beta1.ClusterConfig
	var bootstrapConfig *v1beta1.ClusterConfig
	if o.initialConfig != nil {
		initialConfig = o.initialConfig.DeepCopy()
	}
	if o.bootstrapConfig != nil {
		bootstrapConfig = o.bootstrapConfig.DeepCopy()
	}

	return CLIOptions{
		WorkerOptions:     o.WorkerOptions.Copy(),
		ControllerOptions: o.ControllerOptions.Copy(),
		CfgFile:           o.CfgFile,
		DataDir:           o.DataDir,
		Debug:             o.Debug,
		DebugListenOn:     o.DebugListenOn,
		StatusSocket:      o.StatusSocket,
		Verbose:           o.Verbose,
		K0sVars:           o.K0sVars,
		AutopilotRoot:     o.AutopilotRoot,
		initialConfig:     initialConfig,
		bootstrapConfig:   bootstrapConfig,
	}
}

// Shared controller cli flags
type ControllerOptions struct {
	EnableWorker      bool
	SingleNode        bool
	NoTaints          bool
	DisableComponents []string

	EnableK0sCloudProvider          bool
	K0sCloudProviderPort            int
	K0sCloudProviderUpdateFrequency time.Duration
	EnableDynamicConfig             bool
	EnableMetricsScraper            bool
	KubeControllerManagerExtraArgs  string
}

func (c *ControllerOptions) Copy() ControllerOptions {
	disableComponents := make([]string, len(c.DisableComponents))
	copy(disableComponents, c.DisableComponents)

	return ControllerOptions{
		EnableWorker:                    c.EnableWorker,
		SingleNode:                      c.SingleNode,
		NoTaints:                        c.NoTaints,
		DisableComponents:               disableComponents,
		EnableK0sCloudProvider:          c.EnableK0sCloudProvider,
		K0sCloudProviderPort:            c.K0sCloudProviderPort,
		K0sCloudProviderUpdateFrequency: c.K0sCloudProviderUpdateFrequency,
		EnableDynamicConfig:             c.EnableDynamicConfig,
		EnableMetricsScraper:            c.EnableMetricsScraper,
		KubeControllerManagerExtraArgs:  c.KubeControllerManagerExtraArgs,
	}
}

// Shared worker cli flags
type WorkerOptions struct {
	APIServer        string
	CIDRRange        string
	CloudProvider    bool
	ClusterDNS       string
	LogLevels        map[string]string
	CriSocket        string
	KubeletExtraArgs string
	Labels           []string
	Taints           []string
	TokenFile        string
	TokenArg         string
	WorkerProfile    string
	IPTablesMode     string
}

func (w *WorkerOptions) Copy() WorkerOptions {
	labels := make([]string, len(w.Labels))
	taints := make([]string, len(w.Taints))
	logLevels := make(map[string]string)
	copy(labels, w.Labels)
	copy(taints, w.Taints)

	// Copy the values from the original map to the new map
	for k, v := range w.LogLevels {
		logLevels[k] = v
	}

	// Create a new instance of WorkerOptions with a new map and new slices
	return WorkerOptions{
		APIServer:        w.APIServer,
		CIDRRange:        w.CIDRRange,
		CloudProvider:    w.CloudProvider,
		ClusterDNS:       w.ClusterDNS,
		LogLevels:        logLevels,
		CriSocket:        w.CriSocket,
		KubeletExtraArgs: w.KubeletExtraArgs,
		Labels:           labels,
		Taints:           taints,
		TokenFile:        w.TokenFile,
		TokenArg:         w.TokenArg,
		WorkerProfile:    w.WorkerProfile,
		IPTablesMode:     w.IPTablesMode,
	}
}

func (o *ControllerOptions) Normalize() error {
	// Normalize component names
	var disabledComponents []string
	for _, disabledComponent := range o.DisableComponents {
		switch disabledComponent {
		case constant.APIConfigComponentName:
			logrus.Warnf("Usage of deprecated component name %q, please switch to %q",
				constant.APIConfigComponentName, "--enable-dynamic-config=false",
			)
			if o.EnableDynamicConfig {
				logrus.Warnf("Cannot disable component %q, because %q is selected",
					constant.APIConfigComponentName, "--enable-dynamic-config",
				)
			}

		case constant.KubeletConfigComponentName:
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

func GetPersistentFlagSet() *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	flagset.BoolVarP(&Debug, "debug", "d", false, "Debug logging (default: false)")
	flagset.BoolVarP(&Verbose, "verbose", "v", false, "Verbose logging (default: false)")
	flagset.StringVar(&DataDir, "data-dir", "", "Data Directory for k0s (default: /var/lib/k0s). DO NOT CHANGE for an existing setup, things will break!")
	flagset.StringVar(&StatusSocket, "status-socket", "", "Full file path to the socket file. (default: <rundir>/status.sock)")
	flagset.StringVar(&DebugListenOn, "debugListenOn", ":6060", "Http listenOn for Debug pprof handler")
	return flagset
}

// XX: not a pretty hack, but we need the data-dir flag for the kubectl subcommand
// XX: when other global flags cannot be used (specifically -d and -c)
func GetKubeCtlFlagSet() *pflag.FlagSet {
	debugDefault := false
	if v, ok := os.LookupEnv("DEBUG"); ok {
		debugDefault, _ = strconv.ParseBool(v)
	}

	flagset := &pflag.FlagSet{}
	flagset.StringVar(&DataDir, "data-dir", "", "Data Directory for k0s (default: /var/lib/k0s). DO NOT CHANGE for an existing setup, things will break!")
	flagset.BoolVar(&Debug, "debug", debugDefault, "Debug logging [$DEBUG]")
	return flagset
}

func GetCriSocketFlag() *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	flagset.StringVar(&workerOpts.CriSocket, "cri-socket", "", "container runtime socket to use, default to internal containerd. Format: [remote|docker]:[path-to-socket]")
	return flagset
}

func GetWorkerFlags() *pflag.FlagSet {
	flagset := &pflag.FlagSet{}

	flagset.StringVar(&workerOpts.WorkerProfile, "profile", "default", "worker profile to use on the node")
	flagset.StringVar(&workerOpts.APIServer, "api-server", "", "HACK: api-server for the windows worker node")
	flagset.StringVar(&workerOpts.CIDRRange, "cidr-range", "10.96.0.0/12", "HACK: cidr range for the windows worker node")
	flagset.StringVar(&workerOpts.ClusterDNS, "cluster-dns", "10.96.0.10", "HACK: cluster dns for the windows worker node")
	flagset.BoolVar(&workerOpts.CloudProvider, "enable-cloud-provider", false, "Whether or not to enable cloud provider support in kubelet")
	flagset.StringVar(&workerOpts.TokenFile, "token-file", "", "Path to the file containing token.")
	flagset.StringToStringVarP(&workerOpts.LogLevels, "logging", "l", DefaultLogLevels(), "Logging Levels for the different components")
	flagset.StringSliceVarP(&workerOpts.Labels, "labels", "", []string{}, "Node labels, list of key=value pairs")
	flagset.StringSliceVarP(&workerOpts.Taints, "taints", "", []string{}, "Node taints, list of key=value:effect strings")
	flagset.StringVar(&workerOpts.KubeletExtraArgs, "kubelet-extra-args", "", "extra args for kubelet")
	flagset.StringVar(&workerOpts.IPTablesMode, "iptables-mode", "", "iptables mode (valid values: nft, legacy, auto). default: auto")
	flagset.AddFlagSet(GetCriSocketFlag())

	return flagset
}

var availableComponents = []string{
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

func GetControllerFlags() *pflag.FlagSet {
	flagset := &pflag.FlagSet{}

	flagset.StringVar(&workerOpts.WorkerProfile, "profile", "default", "worker profile to use on the node")
	flagset.BoolVar(&controllerOpts.EnableWorker, "enable-worker", false, "enable worker (default false)")
	flagset.StringSliceVar(&controllerOpts.DisableComponents, "disable-components", []string{}, "disable components (valid items: "+strings.Join(availableComponents, ",")+")")
	flagset.StringVar(&workerOpts.TokenFile, "token-file", "", "Path to the file containing join-token.")
	flagset.StringToStringVarP(&workerOpts.LogLevels, "logging", "l", DefaultLogLevels(), "Logging Levels for the different components")
	flagset.BoolVar(&controllerOpts.SingleNode, "single", false, "enable single node (implies --enable-worker, default false)")
	flagset.BoolVar(&controllerOpts.NoTaints, "no-taints", false, "disable default taints for controller node")
	flagset.BoolVar(&controllerOpts.EnableK0sCloudProvider, "enable-k0s-cloud-provider", false, "enables the k0s-cloud-provider (default false)")
	flagset.DurationVar(&controllerOpts.K0sCloudProviderUpdateFrequency, "k0s-cloud-provider-update-frequency", 2*time.Minute, "the frequency of k0s-cloud-provider node updates")
	flagset.IntVar(&controllerOpts.K0sCloudProviderPort, "k0s-cloud-provider-port", k0scloudprovider.DefaultBindPort, "the port that k0s-cloud-provider binds on")
	flagset.AddFlagSet(GetCriSocketFlag())
	flagset.BoolVar(&controllerOpts.EnableDynamicConfig, "enable-dynamic-config", false, "enable cluster-wide dynamic config based on custom resource")
	flagset.BoolVar(&controllerOpts.EnableMetricsScraper, "enable-metrics-scraper", false, "enable scraping metrics from the controller components (kube-scheduler, kube-controller-manager)")
	flagset.StringVar(&controllerOpts.KubeControllerManagerExtraArgs, "kube-controller-manager-extra-args", "", "extra args for kube-controller-manager")
	flagset.AddFlagSet(FileInputFlag())
	return flagset
}

// The config flag used to be a persistent, joint flag to all commands
// now only a few commands use it. This function helps to share the flag with multiple commands without needing to define
// it in multiple places
func FileInputFlag() *pflag.FlagSet {
	flagset := &pflag.FlagSet{}
	descString := fmt.Sprintf("config file, use '-' to read the config from stdin (default \"%s\")", constant.K0sConfigPathDefault)
	flagset.StringVarP(&CfgFile, "config", "c", "", descString)

	return flagset
}

func DefaultCLIOptions() CLIOptions {
	o := CLIOptions{
		ControllerOptions: controllerOpts,
		WorkerOptions:     workerOpts,

		CfgFile:       CfgFile,
		DataDir:       DataDir,
		Debug:         Debug,
		Verbose:       Verbose,
		DebugListenOn: DebugListenOn,
	}

	o.K0sVars = constant.GetConfig(o.DataDir)

	if StatusSocket != "" {
		o.StatusSocket = StatusSocket
	} else {
		o.StatusSocket = filepath.Join(o.K0sVars.RunDir, "status.sock")
	}

	if o.ControllerOptions.SingleNode {
		o.ControllerOptions.EnableWorker = true
		o.K0sVars.DefaultStorageType = "kine"
	}

	return o
}

func GetCmdOpts(cmd *cobra.Command) CLIOptions {
	o := DefaultCLIOptions()
	o.stdin = cmd.InOrStdin()

	return o
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

func (o *CLIOptions) storageSpec() *v1beta1.StorageSpec {
	switch o.K0sVars.DefaultStorageType {
	case v1beta1.KineStorageType:
		return v1beta1.KineStorageSpec(o.K0sVars.DataDir)
	case v1beta1.EtcdStorageType:
		return v1beta1.EtcdStorageSpec()
	default:
		return v1beta1.DefaultStorageSpec()
	}
}

func (o *CLIOptions) getConfigFile() *v1beta1.ClusterConfig {
	switch o.CfgFile {
	case "":
		logrus.Fatal("config file not specified")
		//exits
	case "-":
		if o.stdin == nil {
			logrus.Fatal("stdin is not available")
			// exits
		}
		cfg, err := v1beta1.ConfigFromReader(o.stdin, o.storageSpec())
		if err != nil {
			logrus.WithError(err).Fatal("can't read config from stdin")
			// exits
		}
		return cfg
	case constant.K0sConfigPathDefault:
		fd, err := os.Open(o.CfgFile)
		if err != nil {
			logrus.WithError(err).Debugf("cannot access default config file (%s), generating default config", o.CfgFile)
			return v1beta1.DefaultClusterConfig(o.storageSpec())
		}
		defer fd.Close()
		cfg, err := v1beta1.ConfigFromReader(fd, o.storageSpec())
		if err != nil {
			logrus.WithError(err).Fatalf("cannot parse default config file (%s)", o.CfgFile)
			// exits
		}
		return cfg
	default:
		fd, err := os.Open(o.CfgFile)
		if err != nil {
			logrus.WithError(err).Fatalf("cannot access config file (%s)", o.CfgFile)
			// exits
		}
		defer fd.Close()
		cfg, err := v1beta1.ConfigFromReader(fd, o.storageSpec())
		if err != nil {
			logrus.WithError(err).Fatalf("cannot parse config file (%s)", o.CfgFile)
		}
		return cfg
	}
	return nil // unreachable
}

// InitialConfig is the configuration as read from the config file or stdin or generated from defaults on startup
func (o *CLIOptions) InitialConfig() *v1beta1.ClusterConfig {
	if o.initialConfig == nil {
		o.initialConfig = o.getConfigFile()
	}
	return o.initialConfig
}

// BootstrapConfig returns the minimal config required to bootstrap the cluster, the rest of the config can come from the dynamic config. Built from the initial config.
func (o *CLIOptions) BootstrapConfig() *v1beta1.ClusterConfig {
	if o.bootstrapConfig == nil {
		o.bootstrapConfig = o.InitialConfig().GetBootstrappingConfig()
	}
	return o.bootstrapConfig
}
