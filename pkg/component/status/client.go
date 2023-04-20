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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/avast/retry-go"
	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0s/pkg/component/prober"
	"github.com/k0sproject/k0s/pkg/constant"
	"github.com/sirupsen/logrus"
)

type K0sStatus struct {
	Version                     string
	Pid                         int
	PPid                        int
	Role                        string
	SysInit                     string
	StubFile                    string
	Output                      string
	Workloads                   bool
	SingleNode                  bool
	DynamicConfig               bool
	Args                        []string
	WorkerToAPIConnectionStatus ProbeStatus
	BootstrapConfig             *v1beta1.ClusterConfig
	ClusterConfig               *v1beta1.ClusterConfig
	K0sVars                     constant.CfgVars
}

type ProbeStatus struct {
	Message string
	Success bool
}

// GetStatus returns the status of the k0s process using the status socket
func GetStatusInfo(ctx context.Context, socketPath string) (*K0sStatus, error) {
	status := &K0sStatus{}
	err := retry.Do(
		func() error {
			err := statusSocketRequest(ctx, socketPath, "status", status)
			return err
		},
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.Delay(250*time.Millisecond),
		retry.OnRetry(func(attempt uint, err error) {
			logrus.Debugf("retrying status query (%d): %v", attempt, err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}
	return status, nil
}

// GetComponentStatus returns the per-component events and health-checks
func GetComponentStatus(socketPath string, maxCount int) (*prober.State, error) {
	status := &prober.State{}
	if err := statusSocketRequest(context.Background(), socketPath,
		fmt.Sprintf("components?maxCount=%d", maxCount),
		status); err != nil {
		return nil, err
	}
	return status, nil
}

func statusSocketRequest(ctx context.Context, socketPath string, path string, tgt interface{}) error {
	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				dc, err := dialer.DialContext(ctx, "unix", socketPath)
				if err != nil {
					return nil, fmt.Errorf("dialcontex: %w", err)
				}
				return dc, nil
			},
		},
	}

	response, err := httpc.Get("http://127.0.0.1/" + path)
	if err != nil {
		return fmt.Errorf("http get: %v %v: %w", socketPath, path, err)
	}
	defer response.Body.Close()

	logrus.Debugf("response code: %d", response.StatusCode)

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		logrus.Debugf("status query response body: %s", body)
		return fmt.Errorf("status: unexpected http status code: %v %v", socketPath, path)
	}

	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(tgt); err != nil {
		return fmt.Errorf("can't decode json: %w", err)
	}
	return nil
}
