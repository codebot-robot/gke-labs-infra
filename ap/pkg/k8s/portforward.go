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
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
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

func (t *PortForwardTask) Run(ctx context.Context, scope *tasks.APScope) error {
	if IsInCluster() {
		klog.Infof("Running in-cluster, skipping port-forward to %s/%s", t.Namespace, t.Service)
		return t.Child.Run(ctx, scope)
	}

	klog.Infof("Starting port-forward to %s/%s (%d:%d)...", t.Namespace, t.Service, t.LocalPort, t.RemotePort)

	pfCmd := exec.CommandContext(ctx, "kubectl", "port-forward",
		"-n", t.Namespace,
		"svc/"+t.Service,
		fmt.Sprintf("%d:%d", t.LocalPort, t.RemotePort))

	var stdout, stderr bytes.Buffer
	pfCmd.Stdout = &stdout
	pfCmd.Stderr = &stderr

	if err := pfCmd.Start(); err != nil {
		return fmt.Errorf("failed to start port-forward: %w", err)
	}

	var hasProxy bool
	defer func() {
		if pfCmd.Process != nil {
			pfCmd.Process.Kill()
		}
		if hasProxy {
			klog.Infof("Stopping docker registry proxy container...")
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			exec.CommandContext(cleanupCtx, "docker", "rm", "-f", "ap-registry-proxy").Run()
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
		klog.Errorf("port-forward to %s/%s did not become ready", t.Namespace, t.Service)
		if stdout.Len() > 0 {
			klog.Errorf("kubectl port-forward stdout: %s", stdout.String())
		}
		if stderr.Len() > 0 {
			klog.Errorf("kubectl port-forward stderr: %s", stderr.String())
		}
		return fmt.Errorf("port-forward did not become ready")
	}

	if runtime.GOOS == "darwin" && t.LocalPort == 5000 {
		if _, err := exec.LookPath("docker"); err == nil {
			klog.Infof("macOS detected with port 5000, starting docker registry proxy container...")
			// Clean up any existing proxy container
			exec.CommandContext(ctx, "docker", "rm", "-f", "ap-registry-proxy").Run()

			// Start the proxy container using alpine/socat
			proxyCmd := exec.CommandContext(ctx, "docker", "run", "-d",
				"--name", "ap-registry-proxy",
				"-p", "127.0.0.1:5000:5000",
				"alpine/socat",
				"TCP-LISTEN:5000,fork,reuseaddr", "TCP:host.docker.internal:5000")
			if err := proxyCmd.Run(); err != nil {
				klog.Warningf("Failed to start docker registry proxy: %v. docker push might fail.", err)
			} else {
				hasProxy = true
				klog.Infof("Docker registry proxy container started successfully.")
			}
		}
	}

	klog.Infof("Port-forward ready, running child task %s", t.Child.GetName())
	return t.Child.Run(ctx, scope)
}

func (t *PortForwardTask) GetName() string {
	return fmt.Sprintf("port-forward-%s", t.Child.GetName())
}

func (t *PortForwardTask) GetChildren() []tasks.Task {
	return []tasks.Task{t.Child}
}
