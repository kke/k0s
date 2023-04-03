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
	"io"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"github.com/k0sproject/k0s/internal/pkg/file"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
)

type configurationStep struct {
	clusterConfig      *v1beta1.ClusterConfig
	tmpDir             string
	restoredConfigPath string
	out                io.Writer
}

func newConfigurationStep(clusterConfig *v1beta1.ClusterConfig, tmpDir, restoredConfigPath string, out io.Writer) *configurationStep {
	return &configurationStep{
		clusterConfig:      clusterConfig,
		tmpDir:             tmpDir,
		restoredConfigPath: restoredConfigPath,
		out:                out,
	}
}

func (c configurationStep) Name() string {
	return "k0s-config"
}

func (c configurationStep) Backup() (StepResult, error) {
	cfgFile := path.Join(c.tmpDir, "k0s.yaml")
	cfgContent, err := yaml.Marshal(c.clusterConfig)
	if err != nil {
		return StepResult{}, fmt.Errorf("failed to marshal cluster config: %w", err)
	}
	if err := os.WriteFile(cfgFile, cfgContent, 0o600); err != nil {
		return StepResult{}, fmt.Errorf("failed to write cluster config: %w", err)
	}
	return StepResult{filesForBackup: []string{cfgFile}}, nil
}

func (c configurationStep) Restore(restoreFrom, restoreTo string) error {
	objectPathInArchive := path.Join(restoreFrom, "k0s.yaml")

	if !file.Exists(objectPathInArchive) {
		logrus.Debugf("%s does not exist in the backup file", objectPathInArchive)
		return nil
	}

	logrus.Infof("Previously used k0s.yaml saved under the data directory `%s`", restoreTo)

	if c.restoredConfigPath == "-" {
		f, err := os.Open(objectPathInArchive)
		if err != nil {
			return err
		}
		if f == nil {
			return fmt.Errorf("couldn't get a file handle for %s", c.restoredConfigPath)
		}
		defer f.Close()
		_, err = io.Copy(c.out, f)
		return err
	}

	logrus.Infof("restoring from `%s` to `%s`", objectPathInArchive, c.restoredConfigPath)
	return file.Copy(objectPathInArchive, c.restoredConfigPath)
}
