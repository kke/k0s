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

package clusterconfig

import (
	"context"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/sirupsen/logrus"
)

var _ ConfigSource = (*staticSource)(nil)

type staticSource struct {
	configPubSub
}

func NewStaticSource(staticConfig *v1beta1.ClusterConfig) (ConfigSource, error) {
	return &staticSource{
		config: staticConfig,
	}, nil
}

func (s *staticSource) Release(context.Context) {
	logrus.WithField("component", "static-config-source").Debug("sending static config via channel")
  
	s.resultChan <- s.staticConfig
}

func (*staticSource) Stop() {}

func (s *staticSource) InitialConfig() *v1beta1.ClusterConfig {
	return nil
}
