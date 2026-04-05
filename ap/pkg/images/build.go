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

package images

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/k8s"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"github.com/gke-labs/gke-labs-infra/codestyle/pkg/walker"
	"k8s.io/klog/v2"
)

// DockerBuildTask represents a task to build a single docker image.
type DockerBuildTask struct {
	ImageName    string
	Dockerfile   string
	Root         string
	Push         bool
	BuildkitHost string
}

func (t *DockerBuildTask) Run(ctx context.Context, repoRoot string) error {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return err
	}
	imagePrefix := cfg.ImageRepo()

	tag := os.Getenv("IMAGE_TAG")
	if tag == "" {
		tag = "latest"
	}

	actualImagePrefix := imagePrefix
	if imagePrefix == "images.local" && t.Push {
		if k8s.IsInCluster() {
			actualImagePrefix = "in-cluster-image-registry.in-cluster-image-registry-system.svc.cluster.local:80"
		} else {
			actualImagePrefix = "localhost:5000"
		}
	}

	var fullImageName string
	if actualImagePrefix != "" {
		fullImageName = fmt.Sprintf("%s/%s:%s", actualImagePrefix, t.ImageName, tag)
	} else {
		fullImageName = fmt.Sprintf("%s:%s", t.ImageName, tag)
	}

	if t.BuildkitHost == "" {
		t.BuildkitHost = os.Getenv("BUILDKIT_HOST")
	}

	if t.BuildkitHost != "" {
		return t.runBuildctl(ctx, t.Root, fullImageName, t.Dockerfile, imagePrefix)
	}
	return t.runDocker(ctx, t.Root, fullImageName, t.Dockerfile, imagePrefix)
}

func (t *DockerBuildTask) runBuildctl(ctx context.Context, root, fullImageName, relDockerfilePath, imagePrefix string) error {
	klog.Infof("Building image %s from %s using buildctl", fullImageName, root)
	output := fmt.Sprintf("type=image,name=%s,push=%t", fullImageName, t.Push)
	if t.Push {
		output += ",registry.insecure=true"
	}

	tag := os.Getenv("IMAGE_TAG")
	if tag == "" {
		tag = "latest"
	}

	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=.",
		"--local", "dockerfile=.",
		"--opt", "filename=" + relDockerfilePath,
		"--opt", "build-arg:IMAGE_PREFIX=" + imagePrefix,
		"--opt", "build-arg:IMAGE_TAG=" + tag,
		"--output", output,
	}

	if strings.HasPrefix(t.BuildkitHost, "k8s://") {
		// handle port forward
		parts := strings.Split(strings.TrimPrefix(t.BuildkitHost, "k8s://"), "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid buildkit host: %s, expected k8s://namespace/service", t.BuildkitHost)
		}
		namespace, service := parts[0], parts[1]

		pfTask := &k8s.PortForwardTask{
			Service:   service,
			Namespace: namespace,
			LocalPort: 8888,
			RemotePort: 8888, // Assuming 8888 for buildkit
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		if err := pfTask.Start(ctx); err != nil {
			return err
		}

		os.Setenv("BUILDKIT_HOST", "tcp://localhost:8888")
	} else {
		os.Setenv("BUILDKIT_HOST", t.BuildkitHost)
	}

	cmd := exec.CommandContext(ctx, "buildctl", buildctlArgs...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildctl build failed for %s: %w", t.ImageName, err)
	}
	return nil
}

func (t *DockerBuildTask) runDocker(ctx context.Context, root, fullImageName, relDockerfilePath, imagePrefix string) error {
	klog.Infof("Building image %s from %s using docker", fullImageName, root)

	tag := os.Getenv("IMAGE_TAG")
	if tag == "" {
		tag = "latest"
	}

	args := []string{
		"build",
		"-t", fullImageName,
		"-f", relDockerfilePath,
		"--build-arg", "IMAGE_PREFIX=" + imagePrefix,
		"--build-arg", "IMAGE_TAG=" + tag,
		".",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed for %s: %w", t.ImageName, err)
	}

	if t.Push {
		klog.Infof("Pushing image %s", fullImageName)
		pushCmd := exec.CommandContext(ctx, "docker", "push", fullImageName)
		pushCmd.Dir = root
		pushCmd.Stdout = os.Stdout
		pushCmd.Stderr = os.Stderr
		if err := pushCmd.Run(); err != nil {
			return fmt.Errorf("docker push failed for %s: %w", t.ImageName, err)
		}
	}
	return nil
}

func (t *DockerBuildTask) GetName() string {
	return fmt.Sprintf("docker-build-%s", t.ImageName)
}

func (t *DockerBuildTask) GetChildren() []tasks.Task {
	return nil
}

// BuildTasks returns a task group for building all docker images found in images/<name>/Dockerfile.
func BuildTasks(root string, push bool, buildkitHost string) (tasks.Task, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	if buildkitHost == "k8s" {
		buildkitHost = "k8s://autodeploy-system/buildkit"
	}

	dockerfiles, err := findDockerfiles(root)
	if err != nil {
		return nil, err
	}

	var buildTasks []tasks.Task
	for _, dockerfile := range dockerfiles {
		relPath, err := filepath.Rel(root, dockerfile)
		if err != nil {
			continue
		}

		name := getImageName(relPath)
		if name == "" {
			continue
		}

		buildTasks = append(buildTasks, &DockerBuildTask{
			ImageName:    name,
			Dockerfile:   relPath,
			Root:         root,
			Push:         push,
			BuildkitHost: buildkitHost,
		})
	}

	var rootTask tasks.Task = &tasks.Group{
		Name:  "build-images",
		Tasks: buildTasks,
	}

	if push && cfg.ImageRepo() == "images.local" {
		rootTask = &k8s.PortForwardTask{
			Child:      rootTask,
			Service:    "in-cluster-image-registry",
			Namespace:  "in-cluster-image-registry-system",
			LocalPort:  5000,
			RemotePort: 80,
		}
	}

	return rootTask, nil
}

// Build builds docker images found in images/<name>/Dockerfile.
func Build(ctx context.Context, root string, push bool, buildkitHost string) error {
	t, err := BuildTasks(root, push, buildkitHost)
	if err != nil {
		return err
	}
	return t.Run(ctx, root)
}

// HasImages returns true if there are any images to build under root.
func HasImages(root string) (bool, error) {
	dockerfiles, err := findDockerfiles(root)
	if err != nil {
		return false, err
	}

	for _, dockerfile := range dockerfiles {
		relPath, err := filepath.Rel(root, dockerfile)
		if err != nil {
			continue
		}
		if getImageName(relPath) != "" {
			return true, nil
		}
	}

	return false, nil
}

func findDockerfiles(root string) ([]string, error) {
	ignoreList := walker.NewIgnoreList([]string{".git", "vendor", "node_modules"})

	var dockerfiles []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if ignoreList.ShouldIgnore(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			// If this directory contains a .ap directory, it's a different root, so skip it.
			if _, err := os.Stat(filepath.Join(path, ".ap")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == "Dockerfile" {
			dockerfiles = append(dockerfiles, path)
		}
		return nil
	})
	return dockerfiles, err
}

func getImageName(relPath string) string {
	parts := strings.Split(relPath, string(os.PathSeparator))

	// Look for images/<name>/Dockerfile structure
	for i, part := range parts {
		if part == "images" && i+2 < len(parts) && parts[len(parts)-1] == "Dockerfile" {
			return parts[i+1]
		}
	}
	return ""
}
