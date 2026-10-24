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
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
)

func TestVersionsYAML_Structure(t *testing.T) {
	if len(parsedKindReleases) == 0 {
		t.Fatal("no kind releases parsed from versions.yaml")
	}
	if len(parsedKindReleases) != 5 {
		t.Fatalf("expected exactly 5 releases in versions.yaml, got %d", len(parsedKindReleases))
	}

	for _, rel := range parsedKindReleases {
		if rel.Version == "" {
			t.Errorf("empty kind version in release")
		}
		if len(rel.Assets) != 5 {
			t.Errorf("kind release %s has %d assets, want 5", rel.Version, len(rel.Assets))
		}
		for _, plat := range supportedPlatforms {
			assetName := "kind-" + plat
			digest, ok := rel.Assets[assetName]
			if !ok || digest == "" {
				t.Errorf("kind release %s missing asset %s", rel.Version, assetName)
				continue
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("kind release %s asset %s digest does not have sha256: prefix: %s", rel.Version, assetName, digest)
			}
			if len(digest) != len("sha256:")+64 {
				t.Errorf("kind release %s asset %s digest length is invalid: %s", rel.Version, assetName, digest)
			}
		}
	}
}

func TestLookupKind(t *testing.T) {
	rel, ok := LookupKind("v0.33.0")
	if !ok || rel == nil {
		t.Fatal("expected to find kind v0.33.0")
	}
	if rel.Version != "v0.33.0" {
		t.Errorf("expected version v0.33.0, got %s", rel.Version)
	}

	relNoV, okNoV := LookupKind("0.33.0")
	if !okNoV || relNoV == nil {
		t.Fatal("expected to find kind 0.33.0")
	}

	_, okNone := LookupKind("v0.0.0-nonexistent")
	if okNone {
		t.Error("expected LookupKind for nonexistent version to return false")
	}
}

func TestVersionsYAML_Sorted(t *testing.T) {
	var prevKind string
	for i, rel := range parsedKindReleases {
		if i > 0 {
			curr := rel.Version
			if toolversions.Compare(prevKind, curr) <= 0 {
				t.Errorf("kind versions table is not sorted newest-first: %s is not newer than %s", prevKind, curr)
			}
		}
		prevKind = rel.Version
	}
}

// TestNoGoGenerateDirective verifies that no //go:generate directive exists under
// ap/pkg/tool/kind so that ap generate does not automatically run update-versions in CI.
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
