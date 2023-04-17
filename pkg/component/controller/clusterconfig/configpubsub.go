/*
Copyright 2023 k0s authors

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
	"sync"

	"github.com/k0sproject/k0s/pkg/apis/k0s.k0sproject.io/v1beta1"
	"github.com/sirupsen/logrus"
)

type configPubSub struct {
	config      *v1beta1.ClusterConfig
	subscribers []chan *v1beta1.ClusterConfig
	mu          sync.RWMutex
}

func (p *configPubSub) updateConfig(config *v1beta1.ClusterConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = config
}

func (p *configPubSub) notifySubscribers(ctx context.Context) {
	// acquire a read lock to safely access the resultChans slice
	p.mu.RLock()
	defer p.mu.RUnlock()

	logrus.WithField("component", "config-source").Debug("distributing config update")

	for _, ch := range p.subscribers {
		select {
		case <-ctx.Done():
			logrus.WithField("component", "config-source").Debug("configuration distribution context cancelled")
			return
		case ch <- p.config:
		default:
			logrus.WithField("component", "config-source").Warn("skipping a blocked channel")
		}
	}
}

func (p *configPubSub) ResultChan() <-chan *v1beta1.ClusterConfig {
	ch := make(chan *v1beta1.ClusterConfig, 1)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.subscribers = append(p.subscribers, ch)

	return ch
}

func (p *configPubSub) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ch := range p.subscribers {
		close(ch)
	}

	p.subscribers = nil
}
