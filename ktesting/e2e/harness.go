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

package e2e

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type Harness struct {
	ClusterName  string
	Namespace    string
	t            *testing.T
	Namespaces   []string
	PollInterval time.Duration
}

func (h *Harness) logf(format string, args ...any) {
	if h.t != nil {
		h.t.Helper()
		h.t.Logf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func NewHarness(t *testing.T, clusterName string) *Harness {
	return &Harness{
		ClusterName: clusterName,
		Namespace:   "default",
		t:           t,
	}
}

func (h *Harness) TrackNamespace(namespace string) {
	h.Namespaces = append(h.Namespaces, namespace)
}

func (h *Harness) CreateTempNamespace(prefix string) string {
	h.t.Helper()
	ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	h.RunCommand("kubectl", "create", "namespace", ns)
	h.TrackNamespace(ns)
	h.t.Cleanup(func() {
		if h.t.Failed() {
			return
		}
		h.RunCommand("kubectl", "delete", "namespace", ns, "--ignore-not-found")
	})
	return ns
}

func (h *Harness) CollectArtifacts(testName string) {
	artifactsDir := os.Getenv("ARTIFACTS")
	if artifactsDir == "" {
		return
	}
	h.logf("Collecting artifacts to %s", artifactsDir)

	for _, ns := range h.Namespaces {
		nsDir := filepath.Join(artifactsDir, "tests", testName, "objects", ns)
		if err := os.MkdirAll(nsDir, 0755); err != nil {
			h.logf("failed to create directory %s: %v", nsDir, err)
		}

		pods, err := exec.Command("kubectl", "get", "pods", "-n", ns).Output()
		if err != nil {
			h.logf("failed to get pods in namespace %s: %v", ns, err)
		} else {
			if err := os.WriteFile(filepath.Join(nsDir, "pods.txt"), pods, 0644); err != nil {
				h.logf("failed to write pods.txt in %s: %v", nsDir, err)
			}
		}

		podsYaml, err := exec.Command("kubectl", "get", "pods", "-n", ns, "-o", "yaml").Output()
		if err != nil {
			h.logf("failed to get pods yaml in namespace %s: %v", ns, err)
		} else {
			if err := os.WriteFile(filepath.Join(nsDir, "pods.yaml"), podsYaml, 0644); err != nil {
				h.logf("failed to write pods.yaml in %s: %v", nsDir, err)
			}
		}

		logsDir := filepath.Join(artifactsDir, "tests", testName, "logs", ns)
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			h.logf("failed to create directory %s: %v", logsDir, err)
		}

		podList, err := exec.Command("kubectl", "get", "pods", "-n", ns, "-o", "jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			h.logf("failed to list pods in namespace %s: %v", ns, err)
		} else {
			for _, pod := range strings.Fields(string(podList)) {
				logs, err := exec.Command("kubectl", "logs", pod, "-n", ns, "--all-containers=true").Output()
				if err != nil {
					h.logf("failed to get logs for pod %s in namespace %s: %v", pod, ns, err)
					continue
				}
				if err := os.WriteFile(filepath.Join(logsDir, pod+".log"), logs, 0644); err != nil {
					h.logf("failed to write pod log for %s in %s: %v", pod, logsDir, err)
				}
			}
		}
	}

	clusterDir := filepath.Join(artifactsDir, "tests", testName, "objects", "_cluster")
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		h.logf("failed to create directory %s: %v", clusterDir, err)
	}
	nodes, err := exec.Command("kubectl", "get", "nodes").Output()
	if err != nil {
		h.logf("failed to get nodes: %v", err)
	} else {
		if err := os.WriteFile(filepath.Join(clusterDir, "nodes.txt"), nodes, 0644); err != nil {
			h.logf("failed to write nodes.txt in %s: %v", clusterDir, err)
		}
	}
	nodesYaml, err := exec.Command("kubectl", "get", "nodes", "-o", "yaml").Output()
	if err != nil {
		h.logf("failed to get nodes yaml: %v", err)
	} else {
		if err := os.WriteFile(filepath.Join(clusterDir, "nodes.yaml"), nodesYaml, 0644); err != nil {
			h.logf("failed to write nodes.yaml in %s: %v", clusterDir, err)
		}
	}
}

func (h *Harness) Setup() {
	h.t.Helper()
	// Check if cluster exists
	cmd := exec.Command("kind", "get", "clusters")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), h.ClusterName) {
		h.t.Logf("Cluster %s already exists", h.ClusterName)
		h.RunCommand("kind", "export", "kubeconfig", "--name", h.ClusterName)
	} else {
		h.t.Logf("Creating cluster %s", h.ClusterName)
		h.RunCommand("kind", "create", "cluster", "--name", h.ClusterName)
	}

	// Ensure default namespace is used, avoiding issues with environment-specific defaults
	h.RunCommand("kubectl", "config", "set-context", "--current", "--namespace="+h.Namespace)

	h.t.Cleanup(func() {
		if h.t.Failed() {
			h.CollectArtifacts(h.t.Name())
		}
		h.Teardown()
	})
}

func (h *Harness) Teardown() {
	h.t.Helper()
	h.t.Logf("Deleting cluster %s", h.ClusterName)
	cmd := exec.Command("kind", "delete", "cluster", "--name", h.ClusterName)
	if out, err := cmd.CombinedOutput(); err != nil {
		h.t.Logf("Failed to delete cluster: %v\nOutput: %s", err, out)
	}
}

func (h *Harness) GetGitRoot() string {
	h.t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		h.t.Fatalf("Failed to find git root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func (h *Harness) RunCommand(name string, args ...string) {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		h.t.Fatalf("Command failed: %s %v\nOutput: %s", name, args, out)
	}
}

func (h *Harness) DockerBuild(tag, dockerfile, context string) {
	h.t.Helper()
	h.t.Logf("Building docker image %s", tag)
	h.RunCommand("docker", "build", "-t", tag, "-f", dockerfile, context)
}

func (h *Harness) KindLoad(tag string) {
	h.t.Helper()
	h.t.Logf("Loading image %s into kind", tag)
	h.RunCommand("kind", "load", "docker-image", tag, "--name", h.ClusterName)
}

func (h *Harness) KubectlApplyContent(name, content string, args ...string) {
	h.t.Helper()
	snippet := content
	if len(snippet) > 100 {
		snippet = snippet[:100] + "..."
	}
	h.t.Logf("Applying manifest content for %s:\n%s", name, snippet)
	cmdArgs := append([]string{"apply", "-f", "-"}, args...)
	cmd := exec.Command("kubectl", cmdArgs...)
	cmd.Stdin = bytes.NewBufferString(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		h.t.Fatalf("Failed to apply content for %s: %v\nOutput: %s\nFull manifest:\n%s", name, err, out, content)
	}
}

func (h *Harness) WaitForDeployment(name, namespace string, timeout time.Duration) error {
	h.t.Helper()
	h.t.Logf("Waiting for deployment %s in namespace %s", name, namespace)
	cmd := exec.Command("kubectl", "rollout", "status", "deployment/"+name, "-n", namespace, "--timeout="+timeout.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deployment %s failed to become ready: %v\nOutput: %s", name, err, out)
	}
	return nil
}

func (h *Harness) WaitForStatefulSet(name, namespace string, timeout time.Duration) error {
	h.t.Helper()
	h.t.Logf("Waiting for statefulset %s in namespace %s", name, namespace)
	cmd := exec.Command("kubectl", "rollout", "status", "statefulset/"+name, "-n", namespace, "--timeout="+timeout.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("statefulset %s failed to become ready: %v\nOutput: %s", name, err, out)
	}
	return nil
}

func (h *Harness) WaitForDaemonSet(name, namespace string, timeout time.Duration) error {
	h.t.Helper()
	h.t.Logf("Waiting for daemonset %s in namespace %s", name, namespace)
	cmd := exec.Command("kubectl", "rollout", "status", "daemonset/"+name, "-n", namespace, "--timeout="+timeout.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daemonset %s failed to become ready: %v\nOutput: %s", name, err, out)
	}
	return nil
}

func (h *Harness) DeleteDeployment(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "deployment", name, "-n", namespace, "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete deployment %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) DeleteStatefulSet(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "statefulset", name, "-n", namespace, "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete statefulset %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) DeleteDaemonSet(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "daemonset", name, "-n", namespace, "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete daemonset %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) DeleteService(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "service", name, "-n", namespace, "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete service %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) DeletePod(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "pod", name, "-n", namespace, "--ignore-not-found", "--wait=true")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete pod %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) DeleteJob(name, namespace string) error {
	if h.t != nil {
		h.t.Helper()
	}
	cmd := exec.Command("kubectl", "delete", "job", name, "-n", namespace, "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete job %s in namespace %s: %w\nOutput: %s", name, namespace, err, string(out))
	}
	return nil
}

func (h *Harness) GetPodLogs(labelSelector, namespace string) string {
	h.t.Helper()
	out, err := exec.Command("kubectl", "logs", "-l", labelSelector, "-n", namespace, "--all-containers=true").CombinedOutput()
	if err != nil {
		h.t.Logf("Warning: failed to get logs for selector %s in namespace %s: %v", labelSelector, namespace, err)
		return string(out)
	}
	return string(out)
}

func (h *Harness) GetPodLogsByName(name, namespace string) string {
	h.t.Helper()
	out, err := exec.Command("kubectl", "logs", name, "-n", namespace, "--all-containers=true").CombinedOutput()
	if err != nil {
		h.t.Logf("Warning: failed to get logs for pod %s in namespace %s: %v", name, namespace, err)
		return string(out)
	}
	return string(out)
}

func (h *Harness) GetPodYaml(labelSelector, namespace string) string {
	h.t.Helper()
	out, err := exec.Command("kubectl", "get", "pod", "-l", labelSelector, "-n", namespace, "-o", "yaml").CombinedOutput()
	if err != nil {
		h.t.Logf("Warning: failed to get pod yaml for selector %s in namespace %s: %v", labelSelector, namespace, err)
		return string(out)
	}
	return string(out)
}

func (h *Harness) GetEvents(namespace string) string {
	h.t.Helper()
	cmdArgs := []string{"get", "events", "--sort-by=.lastTimestamp"}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}
	out, err := exec.Command("kubectl", cmdArgs...).CombinedOutput()
	if err != nil {
		h.t.Logf("Warning: failed to get events: %v", err)
		return string(out)
	}
	return string(out)
}

func (h *Harness) WaitForJobSuccess(name, namespace string, timeout time.Duration) error {
	h.t.Helper()
	h.t.Logf("Waiting for job %s to succeed in namespace %s (timeout: %s)", name, namespace, timeout)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timed out waiting for job %s to succeed after %s", name, timeout)
		}
		cmd := exec.Command("kubectl", "get", "job", name, "-n", namespace, "-o", "jsonpath={.status.succeeded}")
		out, err := cmd.Output()
		if err == nil && string(out) == "1" {
			h.t.Logf("Job %s succeeded", name)
			return nil
		}

		cmd = exec.Command("kubectl", "get", "job", name, "-n", namespace, "-o", "jsonpath={.status.failed}")
		out, err = cmd.Output()
		if err == nil && string(out) == "1" {
			return fmt.Errorf("job %s failed", name)
		}

		time.Sleep(2 * time.Second)
	}
}

func isNotFoundError(output string) bool {
	return strings.Contains(output, "NotFound") || strings.Contains(strings.ToLower(output), "not found")
}

func (h *Harness) WaitForPodReady(name, namespace string, timeout time.Duration) error {
	if h.t != nil {
		h.t.Helper()
	}
	h.logf("Waiting for pod %s to be ready in namespace %s", name, namespace)
	start := time.Now()
	pollInterval := h.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	const maxConsecutiveErrors = 3
	consecutiveErrors := 0

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timed out waiting for pod %s to be ready after %s", name, timeout)
		}

		cmd := exec.Command("kubectl", "get", "pod", name, "-n", namespace, "-o", "jsonpath={.status.phase}")
		phase, err := cmd.CombinedOutput()
		if err != nil {
			if isNotFoundError(string(phase)) {
				h.logf("Pod %s not found yet in namespace %s", name, namespace)
				time.Sleep(pollInterval)
				continue
			}
			consecutiveErrors++
			h.logf("Warning: kubectl get pod phase failed for %s in namespace %s (consecutive failures: %d): %v\nOutput: %s", name, namespace, consecutiveErrors, err, phase)
			if consecutiveErrors >= maxConsecutiveErrors {
				return fmt.Errorf("kubectl failed persistently while waiting for pod %s: %w\nOutput: %s", name, err, phase)
			}
			time.Sleep(pollInterval)
			continue
		}

		cmd = exec.Command("kubectl", "get", "pod", name, "-n", namespace, "-o", "jsonpath={.status.containerStatuses[*].ready}")
		ready, err := cmd.CombinedOutput()
		if err != nil {
			if isNotFoundError(string(ready)) {
				h.logf("Pod %s not found yet in namespace %s", name, namespace)
				time.Sleep(pollInterval)
				continue
			}
			consecutiveErrors++
			h.logf("Warning: kubectl get pod ready status failed for %s in namespace %s (consecutive failures: %d): %v\nOutput: %s", name, namespace, consecutiveErrors, err, ready)
			if consecutiveErrors >= maxConsecutiveErrors {
				return fmt.Errorf("kubectl failed persistently while waiting for pod %s: %w\nOutput: %s", name, err, ready)
			}
			time.Sleep(pollInterval)
			continue
		}

		consecutiveErrors = 0
		phaseStr := strings.TrimSpace(string(phase))
		readyStr := strings.TrimSpace(string(ready))
		h.logf("Pod %s phase: %s, ready: %s", name, phaseStr, readyStr)

		if (phaseStr == "Running" || phaseStr == "Succeeded") && !strings.Contains(readyStr, "false") && readyStr != "" {
			h.logf("Pod %s is ready (phase: %s)", name, phaseStr)
			return nil
		}

		time.Sleep(pollInterval)
	}
}

func (h *Harness) RunInPod(podName, namespace string, command ...string) (string, error) {
	h.t.Helper()
	args := append([]string{"exec", podName, "-n", namespace, "--"}, command...)
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
