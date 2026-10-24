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

package kind

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
)

// RunKind executes the pinned kind binary with the specified arguments.
func RunKind(ctx context.Context, version string, args []string, stdout, stderr io.Writer, stdin io.Reader) error {
	kindBin, err := EnsureKind(ctx, version)
	if err != nil {
		return fmt.Errorf("failed to prepare kind: %w", err)
	}

	cmd := exec.CommandContext(ctx, kindBin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	cmd.Env = fileutils.PrependPath(os.Environ(), filepath.Dir(kindBin))

	return cmd.Run()
}
