// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"k8s.io/klog/v2"
)

// PortForwardTask wraps another task and ensures a port-forward is running while it executes.
type PortForwardTask struct {
	Child      tasks.Task
	Service    string
	Namespace  string
	LocalPort  int
	RemotePort int
}

func (t *PortForwardTask) Run(ctx context.Context, root string) error {
	klog.Infof("Starting port-forward to %s/%s (%d:%d)...", t.Namespace, t.Service, t.LocalPort, t.RemotePort)

	pfCmd := exec.CommandContext(ctx, "kubectl", "port-forward",
		"-n", t.Namespace,
		"svc/"+t.Service,
		fmt.Sprintf("%d:%d", t.LocalPort, t.RemotePort))

	if err := pfCmd.Start(); err != nil {
		return fmt.Errorf("failed to start port-forward: %w", err)
	}

	defer func() {
		if pfCmd.Process != nil {
			pfCmd.Process.Kill()
		}
	}()

	// Wait for port-forward to be ready
	ready := false
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", t.LocalPort), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !ready {
		return fmt.Errorf("port-forward did not become ready")
	}

	klog.Infof("Port-forward ready, running child task %s", t.Child.GetName())
	return t.Child.Run(ctx, root)
}

func (t *PortForwardTask) GetName() string {
	return fmt.Sprintf("port-forward-%s", t.Child.GetName())
}

func (t *PortForwardTask) GetChildren() []tasks.Task {
	return []tasks.Task{t.Child}
}
