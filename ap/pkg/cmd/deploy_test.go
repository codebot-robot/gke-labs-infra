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
	"os"
	"path/filepath"
	"testing"
)

func TestBuildDeployCommand(t *testing.T) {
	rootOpt := &RootOptions{}
	cmd := BuildDeployCommand(rootOpt)
	if cmd == nil {
		t.Fatal("BuildDeployCommand returned nil")
	}
	if cmd.Use != "deploy [path...]" {
		t.Errorf("Unexpected use string: %q", cmd.Use)
	}

	skipPushFlag := cmd.Flags().Lookup("skip-push")
	if skipPushFlag == nil {
		t.Fatal("expected --skip-push flag")
	}
	if skipPushFlag.DefValue != "false" {
		t.Errorf("expected --skip-push default to be false, got %q", skipPushFlag.DefValue)
	}

	buildkitHostFlag := cmd.Flags().Lookup("buildkit-host")
	if buildkitHostFlag == nil {
		t.Fatal("expected --buildkit-host flag")
	}
	if buildkitHostFlag.DefValue != "" {
		t.Errorf("expected --buildkit-host default to be empty without env, got %q", buildkitHostFlag.DefValue)
	}
}

func TestBuildDeployCommand_BuildkitHostEnv(t *testing.T) {
	t.Setenv("BUILDKIT_HOST", "k8s://autodeploy-system/buildkit")

	rootOpt := &RootOptions{}
	cmd := BuildDeployCommand(rootOpt)
	if cmd == nil {
		t.Fatal("BuildDeployCommand returned nil")
	}

	flag := cmd.Flags().Lookup("buildkit-host")
	if flag == nil {
		t.Fatal("expected --buildkit-host flag")
	}
	// Flag default is empty; environment fallback is handled by ResolveBuildkitHost during execution.
	if flag.DefValue != "" {
		t.Errorf("expected default %q, got %q", "", flag.DefValue)
	}
}

func TestBuildDeployCommand_ParseFlags(t *testing.T) {
	rootOpt := &RootOptions{}
	cmd := BuildDeployCommand(rootOpt)
	if err := cmd.ParseFlags([]string{"--skip-push", "--buildkit-host=k8s://custom/buildkit"}); err != nil {
		t.Fatalf("ParseFlags failed: %v", err)
	}

	skipPush, err := cmd.Flags().GetBool("skip-push")
	if err != nil || !skipPush {
		t.Errorf("expected skip-push=true, got %v (err: %v)", skipPush, err)
	}

	buildkitHost, err := cmd.Flags().GetString("buildkit-host")
	if err != nil || buildkitHost != "k8s://custom/buildkit" {
		t.Errorf("expected buildkit-host=k8s://custom/buildkit, got %q (err: %v)", buildkitHost, err)
	}
}

func TestRunDeploy_DryRun(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize git repo
	if err := os.MkdirAll(filepath.Join(tempDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, ".ap"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".ap", "ap.yaml"), []byte("imagePrefix: images.local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "images", "testapp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "images", "testapp", "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opt := DeployOptions{
		RootOptions: &RootOptions{
			RepoRoot: tempDir,
			APRoots:  []string{tempDir},
			DryRun:   true,
		},
		BuildkitHost: "k8s://autodeploy-system/buildkit",
	}

	if err := RunDeploy(t.Context(), opt); err != nil {
		t.Fatalf("RunDeploy dry-run failed: %v", err)
	}
}
