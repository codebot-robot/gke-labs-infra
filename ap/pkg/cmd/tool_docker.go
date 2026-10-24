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

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/docker"
	"github.com/spf13/cobra"
)

// ToolDockerOptions holds configuration for "tool docker".
type ToolDockerOptions struct {
	*RootOptions
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

// BuildToolDockerCommand constructs the cobra command for "tool docker".
func BuildToolDockerCommand(rootOpt *RootOptions) *cobra.Command {
	opt := ToolDockerOptions{
		RootOptions: rootOpt,
	}

	cmd := &cobra.Command{
		Use:                "docker [--] [args...]",
		Short:              "Run docker with pinned binary and buildx plugin",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, rawArgs []string) error {
			raw := rawArgs
			dryRun := opt.DryRun
			if len(raw) > 0 && raw[0] == "--dry-run" {
				dryRun = true
				raw = raw[1:]
			}
			if len(raw) > 0 && raw[0] == "--" {
				raw = raw[1:]
			}

			opt.DryRun = dryRun
			opt.Args = raw
			opt.Stdout = cmd.OutOrStdout()
			opt.Stderr = cmd.ErrOrStderr()
			opt.Stdin = cmd.InOrStdin()
			return RunToolDocker(cmd.Context(), opt)
		},
	}

	return cmd
}

// RunToolDocker executes the docker tool with pinned version and settings.
func RunToolDocker(ctx context.Context, opt ToolDockerOptions) error {
	workDir := opt.APRoot
	if workDir == "" {
		workDir = opt.RepoRoot
	}
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	cfg, err := config.LoadDockerConfig(workDir)
	if err != nil {
		return err
	}
	if cfg == nil && opt.RepoRoot != "" && opt.RepoRoot != workDir {
		cfg, err = config.LoadDockerConfig(opt.RepoRoot)
		if err != nil {
			return err
		}
	}
	if cfg == nil {
		cfg = config.DefaultDockerConfig()
	}

	stdout := opt.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opt.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := opt.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	if opt.DryRun {
		if len(opt.Args) > 0 && opt.Args[0] == "buildx" {
			buildxBin, err := docker.EnsureDockerBuildx(ctx, cfg.Buildx)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "[dry-run] %s %s\n", buildxBin, strings.Join(opt.Args[1:], " "))
			return err
		}

		dockerBin, err := docker.EnsureDocker(ctx, cfg.Docker)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "[dry-run] %s %s\n", dockerBin, strings.Join(opt.Args, " "))
		return err
	}

	return docker.RunDocker(ctx, cfg.Docker, cfg.Buildx, opt.Args, stdout, stderr, stdin)
}
