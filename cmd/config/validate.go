/*
Copyright 2020 k0s authors

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

	"github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	"github.com/k0sproject/k0s/pkg/config"
	"go.uber.org/multierr"

	"github.com/spf13/cobra"
)

func NewValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate k0s configuration",
		Long: `Example:
   k0s config validate --config path_to_config.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var reader io.Reader

			cfgFile, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("%w: --config is required", err)
			}

			switch cfgFile {
			case "-":
				reader = os.Stdin
			case "":
				return fmt.Errorf("--config is required")
			default:
				f, err := os.Open(cfgFile)
				if err != nil {
					return err
				}
				defer f.Close()
				reader = f
			}

			cfg, err := v1beta1.ConfigFromReader(reader)
			if err != nil {
				return err
			}

			errs := cfg.Validate()
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed: %v", multierr.Combine(errs...))
			}

			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())
	cmd.Flags().AddFlagSet(config.FileInputFlag())
	return cmd
}
