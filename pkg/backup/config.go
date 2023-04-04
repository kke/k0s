//go:build !windows
// +build !windows

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

package backup

import (
	"fmt"
	"os"
	"path"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"

	"sigs.k8s.io/yaml"
)

type configurationStep struct {
	tmpDir             string
	config             *v1beta1.ClusterConfig
	restoredConfigPath string
}

func newConfigurationFileStep(config *v1beta1.ClusterConfig, tmpDir string) *configurationStep {
	return &configurationStep{
		config: config,
		tmpDir: tmpDir,
	}
}

func (c configurationStep) Name() string {
	return "k0s-config"
}

func (c configurationStep) Backup() (BackupStepResult, error) {
	cfgFile := path.Join(c.tmpDir, "k0s.yaml")
	data, err := yaml.Marshal(c.config)
	if err != nil {
		return BackupStepResult{}, fmt.Errorf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(cfgFile, data, 0640); err != nil {
		return BackupStepResult{}, fmt.Errorf("failed to write config backup to %s: %v", cfgFile, err)
	}
	return BackupStepResult{filesForBackup: []string{cfgFile}}, nil
}
