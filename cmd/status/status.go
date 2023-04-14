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

package status

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"text/template"

	"github.com/k0sproject/k0s/pkg/component/status"
	"github.com/k0sproject/k0s/pkg/config"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

func NewStatusCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Get k0s instance status information",
		Example: `The command will return information about system init, PID, k0s role, kubeconfig and similar.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := config.GetCmdOpts(cmd)

			cmd.SilenceUsage = true

			if runtime.GOOS == "windows" {
				return fmt.Errorf("currently not supported on windows")
			}

			statusInfo, err := status.GetStatusInfo(cmd.Context(), c.StatusSocket)
			if err != nil {
				return err
			}
			if statusInfo != nil {
				return printStatus(cmd.OutOrStdout(), statusInfo, output)
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "K0s is not running")
			}
			return nil
		},
	}

	cmd.SilenceUsage = true
	cmd.PersistentFlags().StringVarP(&output, "out", "o", "", "sets type of output to json or yaml")
	cmd.PersistentFlags().StringVar(&config.StatusSocket, "status-socket", "", "Full file path to the socket file. (default: <rundir>/status.sock)")
	cmd.AddCommand(NewStatusSubCmdComponents())
	return cmd
}

func NewStatusSubCmdComponents() *cobra.Command {
	var maxCount int
	cmd := &cobra.Command{
		Use:     "components",
		Short:   "Get k0s instance component status information",
		Example: `The command will return information about k0s components.`,
		PreRun: func(cmd *cobra.Command, args []string) {
			logrus.SetOutput(cmd.ErrOrStderr())
			logrus.Warn("all of the components do not provide full status reports yet, the output of this command may be inaccurate")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := config.GetCmdOpts(cmd)

			cmd.SilenceUsage = true
			if runtime.GOOS == "windows" {
				return fmt.Errorf("currently not supported on windows")
			}
			state, err := status.GetComponentStatus(c.StatusSocket, maxCount)
			if err != nil {
				return err
			}
			d, err := yaml.Marshal(state)
			if err != nil {
				return err
			}
			fmt.Println(string(d))
			return nil
		},
	}
	cmd.Flags().IntVar(&maxCount, "max-count", 1, "how many latest probes to show")
	return cmd

}

const statusTextTemplate = `Version: {{.Version}}
Process ID: {{.Pid}}
Role: {{.Role}}
Workloads: {{.Workloads}}
{{- if eq .Role "controller"}}
SingleNode: {{.SingleNode}}
Dynamic Config: {{.DynamicConfig}}
{{- end}}
{{- if .Workloads}}
Kube-api probing successful: {{.WorkerToAPIConnectionStatus.Success}}
Kube-api probing last error: {{.WorkerToAPIConnectionStatus.Message}}
{{- end}}
{{- if .SysInit}}
Init System: {{.SysInit}}
{{- end}}
{{- if .StubFile}}
Service file: {{.StubFile}}
{{- end}}`

func printStatus(w io.Writer, status *status.K0sStatus, output string) error {
	switch output {
	case "json":
		jsn, err := json.MarshalIndent(status, "", "   ")
		if err != nil {
			return fmt.Errorf("json encode status info: %w", err)
		}
		fmt.Fprintln(w, string(jsn))
	case "yaml":
		ym, err := yaml.Marshal(status)
		if err != nil {
			return fmt.Errorf("yaml encode status info: %w", err)
		}
		fmt.Fprintln(w, string(ym))
	default:
		tmpl, err := template.New("status").Parse(statusTextTemplate)
		if err != nil {
			return fmt.Errorf("error parsing template: %w", err)
		}
		if err := tmpl.Execute(w, status); err != nil {
			return fmt.Errorf("error rendering status template: %w", err)
		}
	}
	return nil
}
