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

func NewEditCmd() *cobra.Command {
	// since the status edit command re-executes itself with a different set of arguments
	// it's theoretically possible to end up in an infinite loop, this bool is used
	// to detect that has happened and exit with an error
	var recursionGuard atomic.Bool

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Launch the editor configured in your shell to edit k0s configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if recursionGuard.Swap(true) {
				logrus.Fatalf("status edit command fatal malfunction")
			}

			c := config.GetCmdOpts(cmd)

			cmd.SetArgs([]string{os.Args[0], "kubectl", "--data-dir", c.DataDir, "-n", "kube-system", "edit", "clusterconfig", "k0s"})

			return cmd.Execute()
		},
	}
	cmd.PersistentFlags().AddFlagSet(config.GetKubeCtlFlagSet())
	return cmd
}
