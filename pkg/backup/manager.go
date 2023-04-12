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
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/k0sproject/k0s/internal/pkg/archive"
	"github.com/k0sproject/k0s/internal/pkg/file"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/constant"
)

// Manager hold configuration for particular backup-restore process
type Manager struct {
	steps   []Backuper
	tmpDir  string
	dataDir string
}

// RunBackup backups cluster
func (bm *Manager) RunBackup(clusterConfig *v1beta1.ClusterConfig, vars constant.CfgVars, savePathDir string, out io.Writer) error {
	bm.discoverSteps(clusterConfig, vars, "backup", "", out)
	defer os.RemoveAll(bm.tmpDir)
	assets := make([]string, 0, len(bm.steps))

	logrus.Info("Starting backup")
	for _, step := range bm.steps {
		logrus.Info("Backup step: ", step.Name())
		result, err := step.Backup()
		if err != nil {
			return fmt.Errorf("failed to create backup on step `%s`: %v", step.Name(), err)
		}
		assets = append(assets, result.filesForBackup...)
	}

	if savePathDir == "-" {
		return createArchive(out, assets, bm.dataDir)
	}

	backupFileName := fmt.Sprintf("k0s_backup_%s.tar.gz", timeStamp())
	if err := bm.save(backupFileName, assets); err != nil {
		return fmt.Errorf("failed to create archive `%s`: %v", backupFileName, err)
	}
	srcBackupFile := filepath.Join(bm.tmpDir, backupFileName)
	destBackupFile := filepath.Join(savePathDir, backupFileName)
	if err := file.Copy(srcBackupFile, destBackupFile); err != nil {
		return fmt.Errorf("failed to rename temporary archive: %v", err)
	}
	logrus.Infof("archive %s created successfully", destBackupFile)
	return nil
}

func (bm *Manager) discoverSteps(clusterConfig *v1beta1.ClusterConfig, vars constant.CfgVars, action string, restoredConfigPath string, out io.Writer) {
	if clusterConfig.Spec.Storage.Type == v1beta1.EtcdStorageType && !clusterConfig.Spec.Storage.Etcd.IsExternal() {
		bm.Add(newEtcdStep(bm.tmpDir, vars.CertRootDir, vars.EtcdCertDir, clusterConfig.Spec.Storage.Etcd.PeerAddress, vars.EtcdDataDir))
	} else if clusterConfig.Spec.Storage.Type == v1beta1.KineStorageType && strings.HasPrefix(clusterConfig.Spec.Storage.Kine.DataSource, "sqlite:") {
		bm.Add(newSqliteStep(bm.tmpDir, clusterConfig.Spec.Storage.Kine.DataSource, vars.DataDir))
	} else {
		logrus.Warnf("only internal etcd and sqlite %s are supported. Other storage backends must be backed-up/restored manually.", action)
	}
	bm.dataDir = vars.DataDir
	for _, path := range []string{
		vars.CertRootDir,
		vars.ManifestsDir,
		vars.OCIBundleDir,
		vars.HelmHome,
		vars.HelmRepositoryConfig,
	} {
		if action == "backup" {
			logrus.Infof("adding `%s` path to the backup archive", path)
		}
		bm.Add(NewFilesystemStep(path))
	}
	bm.Add(newConfigurationStep(clusterConfig, bm.tmpDir, restoredConfigPath, out))
}

// Add adds backup step
func (bm *Manager) Add(step Backuper) {
	if bm.steps == nil {
		bm.steps = []Backuper{step}
		return
	}
	bm.steps = append(bm.steps, step)
}

func (bm Manager) save(backupFileName string, assets []string) error {
	archiveFile := filepath.Join(bm.tmpDir, backupFileName)
	logrus.Debugf("creating temporary archive file: %v", archiveFile)
	out, err := os.Create(archiveFile)
	if err != nil {
		return fmt.Errorf("error creating archive file: %v", err)
	}
	defer out.Close()
	// Create the archive and write the output to the "out" Writer
	err = createArchive(out, assets, bm.dataDir)
	if err != nil {
		logrus.Fatalf("error creating archive: %v", err)
	}

	destinationFile := filepath.Join(bm.tmpDir, backupFileName)
	err = file.Copy(archiveFile, destinationFile)
	if err != nil {
		return fmt.Errorf("failed to copy archive file from temporary directory: %v", err)
	}
	return nil
}

// RunRestore restores cluster
func (bm *Manager) RunRestore(archivePath string, k0sVars constant.CfgVars, desiredRestoredConfigPath string, out io.Writer) error {
	var input io.Reader
	if archivePath == "-" {
		input = os.Stdin
	} else {
		i, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer i.Close()
		input = i
	}
	if err := archive.Extract(input, bm.tmpDir); err != nil {
		return fmt.Errorf("failed to unpack backup archive `%s`: %v", archivePath, err)
	}
	defer os.RemoveAll(bm.tmpDir)
	cfg, err := bm.configFromBackup()
	if err != nil {
		return fmt.Errorf("failed to parse backed-up configuration file, check the backup archive: %v", err)
	}
	bm.discoverSteps(cfg, k0sVars, "restore", desiredRestoredConfigPath, out)
	logrus.Info("Starting restore")

	for _, step := range bm.steps {
		logrus.Info("Restore step: ", step.Name())
		if err := step.Restore(bm.tmpDir, bm.dataDir); err != nil {
			return fmt.Errorf("failed to restore on step `%s`: %v", step.Name(), err)
		}
	}
	return nil
}

func (bm Manager) configFromBackup() (*v1beta1.ClusterConfig, error) {
	cfgFile := path.Join(bm.tmpDir, "k0s.yaml")
	f, err := os.Open(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open config from backup %s: %w", cfgFile, err)
	}
	return v1beta1.ConfigFromReader(f)
}

// NewBackupManager builds new manager
func NewBackupManager() (*Manager, error) {
	tmpDir, err := os.MkdirTemp("", "k0s-backup")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %v", err)
	}

	bm := &Manager{
		tmpDir: tmpDir,
	}

	return bm, nil
}

// Backuper defines interface for backup-restore step
type Backuper interface {
	Name() string
	Backup() (StepResult, error)
	Restore(from, to string) error
}

// StepResult backup result for the particular step
type StepResult struct {
	filesForBackup []string
}
