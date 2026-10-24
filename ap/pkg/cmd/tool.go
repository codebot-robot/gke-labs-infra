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
	"fmt"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/docker"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/kind"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/protoc"
	"github.com/spf13/cobra"
)

// BuildToolCommand constructs the cobra command for "tool".
func BuildToolCommand(rootOpt *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool [subcommand]",
		Short: "Manage and run pinned developer tools",
	}

	cmd.AddCommand(BuildToolProtocCommand(rootOpt))
	cmd.AddCommand(BuildToolDockerCommand(rootOpt))
	cmd.AddCommand(BuildToolDockerBuildxCommand(rootOpt))
	cmd.AddCommand(BuildToolKindCommand(rootOpt))
	cmd.AddCommand(BuildToolUpdateVersionsCommand(rootOpt))

	return cmd
}

// ToolUpdateVersionsOptions holds flags for "tool update-versions".
type ToolUpdateVersionsOptions struct {
	Protoc bool
	Docker bool
	Buildx bool
	Kind   bool
}

// BuildToolUpdateVersionsCommand constructs the cobra command for "tool update-versions".
func BuildToolUpdateVersionsCommand(rootOpt *RootOptions) *cobra.Command {
	opts := ToolUpdateVersionsOptions{
		Protoc: true,
		Docker: true,
		Buildx: true,
		Kind:   true,
	}

	cmd := &cobra.Command{
		Use:   "update-versions",
		Short: "Update baked-in tool release versions and checksums in versions.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot := rootOpt.RepoRoot
			if repoRoot == "" {
				repoRoot = rootOpt.APRoot
			}
			if repoRoot == "" {
				var err error
				repoRoot, _, err = findRoots()
				if err != nil {
					return err
				}
			}

			if opts.Protoc {
				if err := protoc.Update(cmd.Context(), repoRoot); err != nil {
					return fmt.Errorf("failed updating protoc versions: %w", err)
				}
			}

			if opts.Docker || opts.Buildx {
				if err := docker.Update(cmd.Context(), repoRoot, docker.UpdateOptions{
					Docker: opts.Docker,
					Buildx: opts.Buildx,
				}); err != nil {
					return fmt.Errorf("failed updating docker versions: %w", err)
				}
			}

			if opts.Kind {
				if err := kind.Update(cmd.Context(), repoRoot); err != nil {
					return fmt.Errorf("failed updating kind versions: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.Protoc, "protoc", true, "Update protoc and plugin versions in ap/pkg/tool/protoc/versions.yaml")
	cmd.Flags().BoolVar(&opts.Docker, "docker", true, "Update Docker versions in ap/pkg/tool/docker/versions.yaml")
	cmd.Flags().BoolVar(&opts.Buildx, "buildx", true, "Update Docker Buildx versions in ap/pkg/tool/docker/versions.yaml")
	cmd.Flags().BoolVar(&opts.Kind, "kind", true, "Update kind versions in ap/pkg/tool/kind/versions.yaml")

	return cmd
}
