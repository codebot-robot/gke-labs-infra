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

package kindload

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	toolDocker "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/docker"
	"k8s.io/klog/v2"
)

// DefaultBackoffs defines the default retry backoffs for loading images into kind.
var DefaultBackoffs = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	16 * time.Second,
	32 * time.Second,
}

// KindLoadRetryPolicy defines the default backoffs used when retrying kind load docker-image.
var KindLoadRetryPolicy = DefaultBackoffs

// LoadImages loads docker images into the specified kind cluster using the given kind binary.
// It retries on failure with exponential backoff.
func LoadImages(ctx context.Context, kindBin, cluster string, images []string) error {
	return LoadImagesWithBackoffs(ctx, kindBin, cluster, images, DefaultBackoffs)
}

// LoadImagesWithBackoffs loads docker images into the specified kind cluster using the given kind binary and backoff durations.
func LoadImagesWithBackoffs(ctx context.Context, kindBin, cluster string, images []string, backoffs []time.Duration) error {
	if kindBin == "" {
		return fmt.Errorf("kindBin is required")
	}
	if len(images) == 0 {
		return nil
	}
	args := []string{"load", "docker-image"}
	args = append(args, images...)
	if cluster != "" {
		args = append(args, "--name", cluster)
	}

	for attempt := 0; ; attempt++ {
		cmd := exec.CommandContext(ctx, kindBin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = fileutils.PrependPath(os.Environ(), filepath.Dir(kindBin))
		if dockerBin, err := toolDocker.EnsureDocker(ctx, ""); err == nil && dockerBin != "" {
			cmd.Env = fileutils.PrependPath(cmd.Env, filepath.Dir(dockerBin))
		}
		err := cmd.Run()
		if err == nil {
			return nil
		}
		if attempt >= len(backoffs) {
			return fmt.Errorf("failed to load image(s) into kind cluster %q: %w", cluster, err)
		}
		backoff := backoffs[attempt]
		klog.Warningf("failed to load image(s) into kind cluster %q (attempt %d/%d): %v; retrying in %v...", cluster, attempt+1, len(backoffs)+1, err, backoff)
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to load image(s) into kind cluster %q: %w", cluster, ctx.Err())
		case <-time.After(backoff):
		}
	}
}
