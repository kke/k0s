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
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/k0sproject/k0s/internal/pkg/archive"
	"github.com/k0sproject/k0s/internal/pkg/dir"
	"github.com/k0sproject/k0s/internal/pkg/file"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/constant"
)

// Manager hold configuration for particular backup-restore process
type Manager struct {
	vars  constant.CfgVars
	steps []Backuper
}

// Add adds backup step
func (bm *Manager) Add(steps ...Backuper) {
	bm.steps = append(bm.steps, steps...)
}

// RunBackup backups cluster
func (bm *Manager) RunBackup(clusterConfig *v1beta1.ClusterConfig, vars constant.CfgVars, savePathDir string, out io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "k0s-backup")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storageStep := bm.storageStep(tmpDir, clusterConfig.Spec.Storage)
	if storageStep == nil {
		logrus.Warn("only internal etcd and sqlite are supported, other storage backends must be backed-up/restored manually")
	} else {
		bm.Add(storageStep)
	}

	bm.Add(bm.fileSystemSteps(vars)...)
	bm.Add(newConfigurationFileStep(clusterConfig, tmpDir))

	logrus.Info("Starting backup")

	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, step := range bm.steps {
		logrus.Info("Backup step: ", step.Name())
		result, err := step.Backup()
		if err != nil {
			return fmt.Errorf("failed to create backup on step `%s`: %v", step.Name(), err)
		}

		for _, asset := range result.filesForBackup {
			if err := addToArchive(tw, asset, tmpDir); err != nil {
				return fmt.Errorf("failed to add file `%s` to archive: %v", asset, err)
			}
		}
	}

	return nil
}

// RunRestore restores cluster
func (bm *Manager) RunRestore(archiveIn io.Reader, configOut io.Writer) error {
	if err := archive.Extract(input, bm.tmpDir); err != nil {
		return fmt.Errorf("failed to unpack backup archive `%s`: %v", archivePath, err)
	}
	defer os.RemoveAll(bm.tmpDir)

	cfg, err := bm.configFromBackup()
	if err != nil {
		return fmt.Errorf("failed to parse backed-up configuration file, check the backup archive: %v", err)
	}

	logrus.Info("Starting restore")

	for _, step := range bm.steps {
		logrus.Info("Restore step: ", step.Name())
		if err := step.Restore(bm.tmpDir, k0sVars.DataDir); err != nil {
			return fmt.Errorf("failed to restore on step `%s`: %v", step.Name(), err)
		}
	}

	objectPathInArchive := path.Join(restoreFrom, "k0s.yaml")

	if !file.Exists(objectPathInArchive) {
		logrus.Debugf("%s does not exist in the backup file", objectPathInArchive)
		return nil
	}

	// when user has set --config-out=- they want the config to stdout during restore
	if c.restoredConfigPath == "-" {
		f, err := os.Open(objectPathInArchive)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(c.out, f)
		return err
	}

	logrus.Infof("restoring from `%s` to `%s`", objectPathInArchive, c.restoredConfigPath)
	return file.Copy(objectPathInArchive, c.restoredConfigPath)
}

func (bm Manager) configFromBackup() (*v1beta1.ClusterConfig, error) {
	cfgFile := path.Join(bm.tmpDir, "k0s.yaml")
	f, err := os.Open(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file %s from backup: %w", cfgFile, err)
	}

	cfg, err := v1beta1.ConfigFromReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration file %s from backup: %w", cfgFile, err)
	}

	logrus.Debugf("Using k0s.yaml from: %s", cfgFile)

	return cfg, nil
}

// NewBackupManager builds new manager
func NewBackupManager(vars constant.CfgVars) *Manager {
	return &Manager{vars: vars}
}

// Backuper defines interface for a backup step
type Backuper interface {
	Name() string
	Backup() (BackupStepResult, error)
	Restore() error
}

// BackupStepResult backup result for the particular step
type BackupStepResult struct {
	filesForBackup []string
}

func (c *command) restore(path string, out io.Writer) error {
	if err := dir.Init(c.K0sVars.DataDir, constant.DataDirMode); err != nil {
		return err
	}

	mgr, err := backup.NewBackupManager()
	if err != nil {
		return err
	}
	if c.restoredConfigPath == "" {
		c.restoredConfigPath = defaultConfigFileOutputPath(path)
	}
	return mgr.RunRestore(path, c.K0sVars, c.restoredConfigPath, out)
}

// set output config file name and path according to input archive Timestamps
// the default location for the restore operation is the currently running cwd
// this can be override, by using the --config-out flag
func defaultConfigFileOutputPath(archivePath string) string {
	if archivePath == "-" {
		return "-"
	}
	f := filepath.Base(archivePath)
	nameWithoutExt := strings.Split(f, ".")[0]
	fName := strings.TrimPrefix(nameWithoutExt, "k0s_backup_")
	restoredFileName := fmt.Sprintf("k0s_%s.yaml", fName)

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return path.Join(cwd, restoredFileName)
}

func addToArchive(tw *tar.Writer, filename string, baseDir string) error {
	// Get FileInfo about our file providing file size, mode, etc.
	// Open the file which will be written into the archive
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to fetch file info: %v", err)
	}

	if info.IsDir() {
		return fmt.Errorf("file is a directory: %s", filename)
	}

	// Create a tar Header from the FileInfo data
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return fmt.Errorf("failed to create tar header: %v", err)
	}
	rel, err := filepath.Rel(baseDir, filename)
	if err != nil {
		return fmt.Errorf("failed to fetch relative path in tar archive: %v", err)
	}
	header.Name = rel

	// Write file header to the tar archive
	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("failed to write file header to archive: %v", err)
	}

	if !dir.IsDirectory(filename) {
		// Copy file content to tar archive
		_, err = io.Copy(tw, file)
		if err != nil {
			return fmt.Errorf("failed to copy file contents info archive: %v", err)
		}
	}
	return nil
}

func (bm *Manager) fileSystemSteps(vars constant.CfgVars) []Backuper {
	var fsDirs = []string{
		vars.CertRootDir,
		vars.ManifestsDir,
		vars.OCIBundleDir,
		vars.HelmHome,
		vars.HelmRepositoryConfig,
	}
	steps := make([]Backuper, 0, len(fsDirs))

	for _, path := range fsDirs {
		steps = append(steps, NewFilesystemStep(path))
	}

	return steps
}

func (bm *Manager) storageStep(tmpDir string, storage *v1beta1.StorageSpec) Backuper {
	if storage == nil {
		return nil
	}

	switch storage.Type {
	case v1beta1.EtcdStorageType:
		if !storage.Etcd.IsExternalClusterUsed() {
			return newEtcdStep(tmpDir, bm.vars.CertRootDir, bm.vars.EtcdCertDir, storage.Etcd.PeerAddress, bm.vars.EtcdDataDir)
		}
	case v1beta1.KineStorageType:
		if strings.HasPrefix(storage.Kine.DataSource, "sqlite:") {
			return newSqliteStep(tmpDir, storage.Kine.DataSource, bm.vars.DataDir)
		}
	}

	return nil
}
	
backupFileName := fmt.Sprintf("k0s_backup_%s.tar.gz", timeStamp())
	if err := bm.save(backupFileName, vars.DataDir, assets); err != nil {
		return fmt.Errorf("failed to create archive `%s`: %v", backupFileName, err)
	}
}
