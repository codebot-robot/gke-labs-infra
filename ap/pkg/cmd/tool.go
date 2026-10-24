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
	cmd.AddCommand(BuildToolUpdateProtocVersionsCommand(rootOpt))
	cmd.AddCommand(BuildToolDockerCommand(rootOpt))
	cmd.AddCommand(BuildToolDockerBuildxCommand(rootOpt))

	return cmd
}

// BuildToolUpdateProtocVersionsCommand constructs the cobra command for "tool update-protoc-versions".
func BuildToolUpdateProtocVersionsCommand(rootOpt *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "update-protoc-versions",
		Short: "Update well-known protoc releases and plugin versions in versions.yaml",
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
			return protoc.UpdateVersionsFile(cmd.Context(), repoRoot)
		},
	}
}
