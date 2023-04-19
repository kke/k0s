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

package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/k0sproject/k0s/pkg/component/status"
	"github.com/k0sproject/k0s/pkg/config"
	"github.com/k0sproject/k0s/pkg/token"

	"github.com/spf13/cobra"
)

func tokenCreateCmd() *cobra.Command {
	var (
		createTokenRole string
		tokenExpiry     string
		waitCreate      bool

		errRefusingToCreateToken = errors.New("refusing to create token")
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create join token",
		Example: `k0s token create --role worker --expiry 100h //sets expiration time to 100 hours
k0s token create --role worker --expiry 10m  //sets expiration time to 10 minutes
`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return checkTokenRole(createTokenRole)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := config.GetCmdOpts(cmd)

			expiry, err := time.ParseDuration(tokenExpiry)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			if !waitCreate {
				newCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				ctx = newCtx
			}

			statusInfo, err := status.GetStatusInfo(ctx, c.StatusSocket)
			if err != nil {
				return fmt.Errorf("failed to get k0s status: %w", err)
			}
			if statusInfo == nil {
				return errors.New("failed to get k0s status: status info is nil")
			}

			cfg := statusInfo.ClusterConfig

			if cfg == nil {
				return fmt.Errorf("%w: cluster config is nil", errRefusingToCreateToken)
			}

			if statusInfo.SingleNode {
				return fmt.Errorf("%w: cannot join into a single node cluster", errRefusingToCreateToken)
			}

			if createTokenRole == token.RoleController && !cfg.Spec.Storage.IsJoinable() {
				return fmt.Errorf("%w: cannot join controller into current storage", errRefusingToCreateToken)
			}

			bootstrapToken, err := token.CreateKubeletBootstrapToken(cmd.Context(), cfg.Spec.API, c.K0sVars, createTokenRole, expiry)
			if err != nil {
				return fmt.Errorf("failed to create bootstrap token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), bootstrapToken)
			return nil
		},
	}
	// append flags
	cmd.PersistentFlags().AddFlagSet(config.GetPersistentFlagSet())
	cmd.Flags().StringVar(&tokenExpiry, "expiry", "0s", "Expiration time of the token. Format 1.5h, 2h45m or 300ms.")
	cmd.Flags().StringVar(&createTokenRole, "role", "worker", "Either worker or controller")
	cmd.Flags().BoolVar(&waitCreate, "wait", false, "wait forever (default false)")

	return cmd
}
