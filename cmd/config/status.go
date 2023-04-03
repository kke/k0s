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
	"os"

	"sync/atomic"

	"github.com/k0sproject/k0s/pkg/config"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	// since the status command re-executes itself with a different set of arguments
	// it's theoretically possible to end up in an infinite loop, this bool is used
	// to detect that has happened and exit with an error
	var recursionGuard atomic.Bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display dynamic configuration reconciliation status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if recursionGuard.Swap(true) {
				logrus.Fatalf("status command fatal malfunction")
			}

			c := config.GetCmdOpts(cmd)

			newArgs := []string{os.Args[0], "kubectl", "--data-dir", c.DataDir, "-n", "kube-system", "get", "event", "--field-selector", "involvedObject.name=k0s"}

			if outputFormat := cmd.Flags().Lookup("output").Value.String(); outputFormat != "" {
				newArgs = append(os.Args, "-o", outputFormat)
			}

			cmd.SetArgs(newArgs)

			return cmd.Execute()
		},
	}
	cmd.PersistentFlags().AddFlagSet(config.GetKubeCtlFlagSet())
	cmd.Flags().StringP("output", "o", "", "Output format. Must be one of yaml|json")
	return cmd
}
