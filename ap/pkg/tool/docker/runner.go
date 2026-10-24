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

package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
)

// RunDocker executes the pinned docker binary with the specified arguments.
func RunDocker(ctx context.Context, cfg *config.DockerConfig, args []string, stdout, stderr io.Writer, stdin io.Reader) error {
	if cfg == nil {
		cfg = config.DefaultDockerConfig()
	}

	dockerBin, err := EnsureDockerWithBuildx(ctx, cfg.Docker, cfg.Buildx)
	if err != nil {
		return fmt.Errorf("failed to prepare docker: %w", err)
	}

	cmd := exec.CommandContext(ctx, dockerBin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	cmd.Env = fileutils.PrependPath(os.Environ(), filepath.Dir(dockerBin))

	return cmd.Run()
}

// RunDockerBuildx executes the pinned docker-buildx binary with the specified arguments.
func RunDockerBuildx(ctx context.Context, cfg *config.DockerConfig, args []string, stdout, stderr io.Writer, stdin io.Reader) error {
	if cfg == nil {
		cfg = config.DefaultDockerConfig()
	}

	buildxBin, err := EnsureDockerBuildx(ctx, cfg.Buildx)
	if err != nil {
		return fmt.Errorf("failed to prepare docker-buildx: %w", err)
	}

	cmd := exec.CommandContext(ctx, buildxBin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	cmd.Env = fileutils.PrependPath(os.Environ(), filepath.Dir(buildxBin))

	return cmd.Run()
}
