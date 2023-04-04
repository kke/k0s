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

package testutil

import (
	"testing"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/constant"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"
)

var resourceType = metav1.TypeMeta{APIVersion: "k0s.k0sproject.io/v1beta1", Kind: "clusterconfigs"}

type ConfigGetter struct {
	NodeConfig bool
	YamlData   string

	t       *testing.T
	k0sVars constant.CfgVars
}

// NewConfigGetter sets the parameters required to fetch a fake config for testing
func NewConfigGetter(t *testing.T, yamlData string, isNodeConfig bool, k0sVars constant.CfgVars) *ConfigGetter {
	return &ConfigGetter{
		YamlData:   yamlData,
		NodeConfig: isNodeConfig,
		t:          t,
		k0sVars:    k0sVars,
	}
}

// FakeRuntimeConfig takes a yaml construct and returns a config object from a fake runtime config path
func (c *ConfigGetter) FakeConfig() *v1beta1.ClusterConfig {
	cfg, err := v1beta1.ConfigFromString(c.YamlData, c.getStorageSpec())
	require.NoError(c.t, err, "failed to load cluster config")
	return cfg
}

func (c *ConfigGetter) getStorageSpec() *v1beta1.StorageSpec {
	var storage *v1beta1.StorageSpec

	if c.k0sVars.DefaultStorageType == "kine" {
		storage = &v1beta1.StorageSpec{
			Type: v1beta1.KineStorageType,
			Kine: v1beta1.DefaultKineConfig(c.k0sVars.DataDir),
		}
	}
	return storage
}
