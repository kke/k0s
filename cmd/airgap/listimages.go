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

package airgap

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/k0sproject/k0s/pkg/airgap"
	"github.com/k0sproject/k0s/pkg/config"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func NewAirgapListImagesCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:     "list-images",
		Short:   "List image names and version needed for an air-gap install",
		Example: `k0s airgap list-images`,
		PreRun: func(cmd *cobra.Command, _ []string) {
			logrus.SetOutput(cmd.ErrOrStderr())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := config.GetCmdOpts(cmd)

			if runtime.GOOS == "windows" {
				return fmt.Errorf("currently not supported on windows")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			cfg, err := c.DynamicConfig(ctx)
			if err != nil {
				if !errors.Is(err, config.ErrDynamicConfigNotEnabled) {
					logrus.WithError(err).Error("failed to get dynamic config, falling back to local config")
				}
				cfg = c.InitialConfig()
			}

			for _, uri := range airgap.GetImageURIs(cfg.Spec, all) {
				fmt.Fprintln(cmd.OutOrStdout(), uri)
			}

			return nil
		},
	}
	cmd.Flags().AddFlagSet(config.FileInputFlag())
	cmd.Flags().BoolVar(&all, "all", false, "include all images, even if they are not used in the current configuration")
	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())
	return cmd
}
