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

package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/k0sproject/k0s/internal/pkg/dir"
	"github.com/k0sproject/k0s/internal/pkg/file"
	"github.com/k0sproject/k0s/pkg/config"
	"github.com/k0sproject/k0s/pkg/constant"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

func installControllerCmd(installFlags *installFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "controller",
		Short:   "Install k0s controller on a brand-new system. Must be run as root (or with sudo)",
		Aliases: []string{"server"},
		Example: `All default values of controller command will be passed to the service stub unless overridden.

With the controller subcommand you can setup a single node cluster by running:

	k0s install controller --single
	`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			c := config.GetCmdOpts(cmd)
			if err := c.InitialConfig().ValidationError(); err != nil {
				return err
			}

			// config from stdin needs to be stored in a file
			if c.CfgFile == "-" {
				if file.Exists(constant.K0sConfigPathDefault) {
					return fmt.Errorf("config file %s already exists, please remove it first", constant.K0sConfigPathDefault)
				}
				logrus.Infof("configuration from stdin will be installed to %s", constant.K0sConfigPathDefault)
				if err := dir.Init(filepath.Dir(constant.K0sConfigPathDefault), 0640); err != nil {
					return err
				}
				cfgContent, err := yaml.Marshal(c.InitialConfig())
				if err != nil {
					return err
				}

				if err := os.WriteFile(constant.K0sConfigPathDefault, cfgContent, 0640); err != nil {
					return err
				}

				config.CfgFile = constant.K0sConfigPathDefault
				// cmdFlagsToArgs() will use the config path from the flag
				if err := cmd.Flags().Set("config", constant.K0sConfigPathDefault); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := command{config.GetCmdOpts(cmd)}

			if err := c.convertFileParamsToAbsolute(); err != nil {
				cmd.SilenceUsage = true
				return err
			}
			flagsAndVals := []string{"controller"}
			flagsAndVals = append(flagsAndVals, cmdFlagsToArgs(cmd)...)
			if err := c.setup("controller", flagsAndVals, installFlags); err != nil {
				cmd.SilenceUsage = true
				return err
			}
			return nil
		},
	}
	// append flags
	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())
	cmd.Flags().AddFlagSet(config.GetControllerFlags())
	cmd.Flags().AddFlagSet(config.GetWorkerFlags())
	return cmd
}
