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

package kubectl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
)

func TestVersionsYAML_Structure(t *testing.T) {
	if len(parsedKubectlReleases) == 0 {
		t.Fatal("no kubectl releases parsed from versions.yaml")
	}
	if len(parsedKubectlReleases) != 5 {
		t.Fatalf("expected exactly 5 releases in versions.yaml, got %d", len(parsedKubectlReleases))
	}

	for _, rel := range parsedKubectlReleases {
		if rel.Version == "" {
			t.Errorf("empty kubectl version in release")
		}
		if len(rel.Assets) != 5 {
			t.Errorf("kubectl release %s has %d assets, want 5", rel.Version, len(rel.Assets))
		}
		for _, plat := range supportedPlatforms {
			digest, ok := rel.Assets[plat]
			if !ok || digest == "" {
				t.Errorf("kubectl release %s missing asset %s", rel.Version, plat)
				continue
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("kubectl release %s asset %s digest does not have sha256: prefix: %s", rel.Version, plat, digest)
			}
			if len(digest) != len("sha256:")+64 {
				t.Errorf("kubectl release %s asset %s digest length is invalid: %s", rel.Version, plat, digest)
			}
		}
	}
}

func TestLookupKubectl(t *testing.T) {
	rel, ok := LookupKubectl("v1.37.2")
	if !ok || rel == nil {
		t.Fatal("expected to find kubectl v1.37.2")
	}
	if rel.Version != "v1.37.2" {
		t.Errorf("expected version v1.37.2, got %s", rel.Version)
	}

	relNoV, okNoV := LookupKubectl("1.37.2")
	if !okNoV || relNoV == nil {
		t.Fatal("expected to find kubectl 1.37.2")
	}

	_, okNone := LookupKubectl("v0.0.0-nonexistent")
	if okNone {
		t.Error("expected LookupKubectl for nonexistent version to return false")
	}
}

func TestVersionsYAML_Sorted(t *testing.T) {
	var prevKubectl string
	for i, rel := range parsedKubectlReleases {
		if i > 0 {
			curr := rel.Version
			if toolversions.Compare(prevKubectl, curr) <= 0 {
				t.Errorf("kubectl versions table is not sorted newest-first: %s is not newer than %s", prevKubectl, curr)
			}
		}
		prevKubectl = rel.Version
	}
}

// TestNoGoGenerateDirective verifies that no //go:generate directive exists under
// ap/pkg/tool/kubectl so that ap generate does not automatically run update-versions in CI.
func TestNoGoGenerateDirective(t *testing.T) {
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//go:generate") {
				t.Errorf("%s:%d contains unexpected //go:generate directive: %s (update-versions must only be run manually or by scheduled jobs, never on every ap generate / CI run)", path, lineNum+1, trimmed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk package directory: %v", err)
	}
}
