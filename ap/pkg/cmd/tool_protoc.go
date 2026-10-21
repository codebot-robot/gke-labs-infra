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
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/protoc"
	"github.com/spf13/cobra"
)

// ToolProtocOptions holds configuration for "tool protoc".
type ToolProtocOptions struct {
	*RootOptions
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

// BuildToolProtocCommand constructs the cobra command for "tool protoc".
func BuildToolProtocCommand(rootOpt *RootOptions) *cobra.Command {
	opt := ToolProtocOptions{
		RootOptions: rootOpt,
	}

	cmd := &cobra.Command{
		Use:                "protoc [--] [args...]",
		Short:              "Run protoc with pinned binary and language plugins",
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
			return RunToolProtoc(cmd.Context(), opt)
		},
	}

	return cmd
}

// RunToolProtoc executes the protoc tool with the pinned plugins and settings.
func RunToolProtoc(ctx context.Context, opt ToolProtocOptions) error {
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

	cfg, err := config.LoadProtoConfig(workDir)
	if err != nil {
		return err
	}
	if cfg == nil && opt.RepoRoot != "" && opt.RepoRoot != workDir {
		cfg, err = config.LoadProtoConfig(opt.RepoRoot)
		if err != nil {
			return err
		}
	}
	if cfg == nil {
		cfg = config.DefaultProtoConfig()
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
		env, err := protoc.PrepareProtoc(ctx, cfg, opt.Args)
		if err != nil {
			return err
		}
		var fullArgs []string
		fullArgs = append(fullArgs, opt.Args...)
		if env.IncludeDir != "" {
			fullArgs = append(fullArgs, "-I"+env.IncludeDir)
		}
		_, err = fmt.Fprintf(stdout, "[dry-run] %s %s\n", env.ProtocBin, strings.Join(fullArgs, " "))
		return err
	}

	return protoc.RunProtoc(ctx, cfg, workDir, opt.Args, stdout, stderr, stdin)
}
