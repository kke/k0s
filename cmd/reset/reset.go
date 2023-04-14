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

package reset

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/k0sproject/k0s/pkg/cleanup"
	"github.com/k0sproject/k0s/pkg/component/status"
	"github.com/k0sproject/k0s/pkg/config"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type command struct {
	config.CLIOptions
}

func NewResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Uninstall k0s. Must be run as root (or with sudo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("currently not supported on windows")
			}
			opts := config.GetCmdOpts(cmd)
			if err := opts.BootstrapConfig().ValidationError(); err != nil {
				return err
			}
			c := &command{opts}
			return c.reset()
		},
	}
	cmd.SilenceUsage = true
	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())
	cmd.Flags().AddFlagSet(config.GetCriSocketFlag())
	cmd.Flags().AddFlagSet(config.FileInputFlag())
	return cmd
}

func (c *command) reset() error {
	if os.Geteuid() != 0 {
		logrus.Fatal("this command must be run as root!")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	k0sStatus, _ := status.GetStatusInfo(ctx, c.StatusSocket)
	if k0sStatus != nil && k0sStatus.Pid != 0 {
		logrus.Fatal("k0s seems to be running! please stop k0s before reset.")
	}

	// Get Cleanup Config
	cfg, err := cleanup.NewConfig(c.K0sVars, c.BootstrapConfig(), c.WorkerOptions.CriSocket)
	if err != nil {
		return fmt.Errorf("failed to configure cleanup: %v", err)
	}

	err = cfg.Cleanup()
	logrus.Info("k0s cleanup operations done.")
	logrus.Warn("To ensure a full reset, a node reboot is recommended.")

	return err
}
