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

package executor

import (
	"context"
	"os"
	"os/exec"

	"k8s.io/klog/v2"
)

// Runner defines the interface for running 'ap' commands.
type Runner interface {
	RunAP(ctx context.Context, dir string, args ...string) error
	DeployFlow(ctx context.Context, dir string, args ...string) error
}

// APRunner handles execution of 'ap' commands.
type APRunner struct {
	ImagePrefix  string
	BuildkitHost string
}

// RunAP executes an 'ap' command in the given directory.
func (r *APRunner) RunAP(ctx context.Context, dir string, args ...string) error {
	klog.Infof("Running ap %v in %s", args, dir)

	// TODO: When running in K8s, this should probably create a K8s Job
	// for now we'll just use exec.Command as a placeholder.

	cmd := exec.CommandContext(ctx, "ap", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if r.ImagePrefix != "" {
		cmd.Env = append(cmd.Env, "IMAGE_PREFIX="+r.ImagePrefix)
	}
	if r.BuildkitHost != "" {
		cmd.Env = append(cmd.Env, "BUILDKIT_HOST="+r.BuildkitHost)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("ap command failed: %v, output: %s", err, string(output))
		return err
	}

	return nil
}

// DeployFlow runs the full build-test-deploy flow.
func (r *APRunner) DeployFlow(ctx context.Context, dir string, args ...string) error {
	if err := r.RunAP(ctx, dir, append([]string{"build"}, args...)...); err != nil {
		return err
	}
	if err := r.RunAP(ctx, dir, append([]string{"test"}, args...)...); err != nil {
		return err
	}
	if err := r.RunAP(ctx, dir, append([]string{"deploy"}, args...)...); err != nil {
		return err
	}
	return nil
}
