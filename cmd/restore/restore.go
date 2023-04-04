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

package restore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/k0sproject/k0s/pkg/backup"
	"github.com/k0sproject/k0s/pkg/component/status"
	"github.com/k0sproject/k0s/pkg/config"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type command config.CLIOptions

func NewRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore filename",
		Short: "restore k0s state from given backup archive. Use '-' as filename to read from stdin. Must be run as root (or with sudo)",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := command(config.GetCmdOpts(cmd))

			if _, err := status.GetStatusInfo(c.StatusSocket); err != nil {
				logrus.Fatal("k0s seems to be running! k0s must be down during the restore operation.")
			}

			reader, err := c.reader(cmd, args[0])
			if err != nil {
				return err
			}

			writer, err := c.writer(cmd, args[0], cmd.Flags().Lookup("config-out").Value.String())
			if err != nil {
				return err
			}

			bm := backup.NewBackupManager()
			return bm.RunRestore(reader, writer)
		},
	}

	cmd.SilenceUsage = true

	cmd.Flags().String("config-out", "", "path for the restored k0s.yaml file, use '-' for stdout. (default: k0s_<archive timestamp>.yaml")
	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())

	return cmd
}

func (c *command) reader(cmd *cobra.Command, archivePath string) (io.Reader, error) {
	if archivePath == "-" {
		return cmd.InOrStdin(), nil
	}

	return os.Open(archivePath)
}

func (c *command) writer(cmd *cobra.Command, archivePath, configOut string) (io.Writer, error) {
	if configOut == "-" {
		logrus.SetOutput(cmd.ErrOrStderr()) // make sure we log to stderr to not mess up the output
		return cmd.OutOrStdout(), nil
	}

	if archivePath == "-" {
		archivePath = "k0s.yaml"
	}

	if match := regexp.MustCompile(`k0s_backup_(.+?)\.tar$`).FindStringSubmatch(filepath.Base(configOut)); len(match) > 1 {
		configOut = fmt.Sprintf("k0s_%s.yaml", match[1])
	}

	f, err := os.Create(configOut)
	if err != nil {
		return nil, err
	}
	return f, nil
}
