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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/k0sproject/k0s/pkg/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	fileYaml = `
apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    externalAddress: file-external-address
  network:
    serviceCIDR: 12.12.12.12/12
    podCIDR: 13.13.13.13/13
    kubeProxy:
      mode: ipvs	
`
)

// Test using config from a yaml file
func TestGetConfigFromFile(t *testing.T) {
	CfgFile = writeConfigFile(t, fileYaml)
	opts := DefaultCLIOptions()
	cfg := opts.BootstrapConfig()

	testCases := []struct {
		name     string
		got      string
		expected string
	}{
		{"API_external_address", cfg.Spec.API.ExternalAddress, "file-external-address"},
		{"Network_ServiceCIDR", cfg.Spec.Network.ServiceCIDR, "12.12.12.12/12"},
		{"Network_KubeProxy_Mode", cfg.Spec.Network.KubeProxy.Mode, "ipvs"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s eq %s", tc.name, tc.expected), func(t *testing.T) {
			if tc.got != tc.expected {
				t.Fatalf("expected to read '%s' for the %s test value. Got: %s", tc.expected, tc.name, tc.got)
			}
		})
	}
}

func TestExternalEtcdConfig(t *testing.T) {
	yamlData := `
spec:
  storage:
    type: etcd
    etcd:
      externalCluster:
        endpoints:
        - http://etcd0:2379
        etcdPrefix: k0s-tenant`

	CfgFile = writeConfigFile(t, yamlData)
	opts := DefaultCLIOptions()
	cfg := opts.BootstrapConfig()

	testCases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Storage_Type", cfg.Spec.Storage.Type, "etcd"},
		{"External_Cluster_Endpoint", cfg.Spec.Storage.Etcd.ExternalCluster.Endpoints[0], "http://etcd0:2379"},
		{"External_Cluster_Prefix", cfg.Spec.Storage.Etcd.ExternalCluster.EtcdPrefix, "k0s-tenant"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s eq %s", tc.name, tc.expected), func(t *testing.T) {
			if tc.got != tc.expected {
				t.Fatalf("expected to read '%s' for the %s test value. Got: %s", tc.expected, tc.name, tc.got)
			}
		})
	}
}

func TestConfigFromDefaults(t *testing.T) {
	CfgFile = constant.K0sConfigPathDefault
	opts := DefaultCLIOptions()
	cfg := opts.BootstrapConfig()

	testCases := []struct {
		name     string
		got      string
		expected string
	}{
		{"API_external_address", cfg.Spec.API.ExternalAddress, ""},
		{"Network_ServiceCIDR", cfg.Spec.Network.ServiceCIDR, "10.96.0.0/12"},
		{"Network_KubeProxy_Mode", cfg.Spec.Network.KubeProxy.Mode, "iptables"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s eq %s", tc.name, tc.expected), func(t *testing.T) {
			if tc.got != tc.expected {
				t.Fatalf("expected to read '%s' for the %s test value. Got: %s", tc.expected, tc.name, tc.got)
			}
		})
	}
}

// Test using node-config from a file when API config is enabled
func TestNodeConfigWithAPIConfig(t *testing.T) {
	cfgFilePath := writeConfigFile(t, fileYaml)
	CfgFile = cfgFilePath

	// if API config is enabled, Nodeconfig will be stripped of any cluster-wide-config settings
	controllerOpts.EnableDynamicConfig = true

	opts := DefaultCLIOptions()
	cfg := opts.BootstrapConfig()

	assert.Equal(t, "file-external-address", cfg.Spec.API.ExternalAddress)
	assert.Equal(t, "12.12.12.12/12", cfg.Spec.Network.ServiceCIDR)
	// PodCIDR is a cluster-wide setting. It shouldn't exist in Node config
	assert.Empty(t, cfg.Spec.Network.PodCIDR)
}

func TestSingleNodeConfig(t *testing.T) {

	yamlData := `
spec:
  api:
    address: 1.2.3.4`

	CfgFile = writeConfigFile(t, yamlData)
	opts := DefaultCLIOptions()

	tempDir := t.TempDir()
	opts.K0sVars = constant.GetConfig(tempDir)
	opts.K0sVars.DefaultStorageType = "kine"

	cfg := opts.BootstrapConfig()

	assert.Equal(t, "kine", cfg.Spec.Storage.Type, "Storage type mismatch")
	assert.Contains(t,
		cfg.Spec.Storage.Kine.DataSource,
		(&url.URL{
			Scheme:   "sqlite",
			OmitHost: true,
			Path:     filepath.ToSlash(filepath.Join(tempDir, "db", "state.db")),
		}).String(),
		"Data source mismatch",
	)
}

func TestSingleNodeConfigWithEtcd(t *testing.T) {
	yamlData := `
spec:
  storage:
    type: etcd`

	CfgFile = writeConfigFile(t, yamlData)
	opts := DefaultCLIOptions()

	opts.K0sVars = constant.GetConfig(t.TempDir())
	opts.K0sVars.DefaultStorageType = "kine"

	cfg := opts.BootstrapConfig()

	testCases := []struct {
		name     string
		got      string
		expected string
	}{{"Storage_Type", cfg.Spec.Storage.Type, "etcd"}} // config file storage type trumps k0sVars.DefaultStorageType

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s eq %s", tc.name, tc.expected), func(t *testing.T) {
			if tc.got != tc.expected {
				t.Fatalf("expected to read '%s' for the %s test value. Got: %s", tc.expected, tc.name, tc.got)
			}
		})
	}
}

// when a component requests an API config,
// the merged node and cluster config should be returned
func TestAPIConfig(_ *testing.T) {
	// todo: not sure how to adapt this test
}

func writeConfigFile(t *testing.T, yamlData string) (filePath string) {
	cfgFilePath := path.Join(t.TempDir(), "k0s-config.yaml")
	require.NoError(t, os.WriteFile(cfgFilePath, []byte(yamlData), 0644))
	return cfgFilePath
}
