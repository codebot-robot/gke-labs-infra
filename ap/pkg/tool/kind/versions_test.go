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
	if len(parsedReleases) < 5 {
		t.Fatalf("expected at least 5 kind releases in versions.yaml, got %d", len(parsedReleases))
	}

	for _, rel := range parsedReleases {
		if rel.Version == "" {
			t.Errorf("empty version in release")
		}
		if len(rel.Assets) != 5 {
			t.Errorf("expected release %s to have 5 assets, got %d", rel.Version, len(rel.Assets))
		}
		for _, assetName := range supportedAssets {
			digest, ok := rel.Assets[assetName]
			if !ok || digest == "" {
				t.Errorf("kind release %s missing asset %s", rel.Version, assetName)
				continue
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("kind release %s asset %s digest missing sha256: prefix: %s", rel.Version, assetName, digest)
			}
			if len(digest) != len("sha256:")+64 {
				t.Errorf("kind release %s asset %s digest length invalid: %s", rel.Version, assetName, digest)
			}
		}
	}
}

func TestVersionsYAML_Sorted(t *testing.T) {
	var prev string
	for i, rel := range parsedReleases {
		if i > 0 {
			curr := rel.Version
			if toolversions.Compare(prev, curr) <= 0 {
				t.Errorf("kind versions table is not sorted newest-first: %s is not newer than %s", prev, curr)
			}
		}
		prev = rel.Version
	}
}

func TestLookup(t *testing.T) {
	rel, ok := Lookup("v0.33.0")
	if !ok || rel == nil {
		t.Fatal("expected to find kind v0.33.0")
	}
	if rel.Version != "v0.33.0" {
		t.Errorf("expected version v0.33.0, got %s", rel.Version)
	}

	relNoV, okNoV := Lookup("0.33.0")
	if !okNoV || relNoV == nil {
		t.Fatal("expected to find kind 0.33.0 without v")
	}

	_, okNone := Lookup("v0.0.0-nonexistent")
	if okNone {
		t.Error("expected Lookup for nonexistent version to return false")
	}
}

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
