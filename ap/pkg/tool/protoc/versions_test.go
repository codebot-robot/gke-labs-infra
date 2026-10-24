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

package protoc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/scoped"
	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
	"gopkg.in/yaml.v3"
)

// TestVersionsTableIntegrity asserts every table entry has digests for all supported assets
// and that the table is sorted newest first.
func TestVersionsTableIntegrity(t *testing.T) {
	if len(parsedVersions.Protoc) == 0 {
		t.Fatal("expected at least one protoc release in versions table")
	}

	for i, rel := range parsedVersions.Protoc {
		if rel.Version == "" {
			t.Errorf("entry %d has empty version", i)
		}

		for _, plat := range supportedPlatforms {
			expectedAsset := fmt.Sprintf("protoc-%s-%s.zip", rel.Version, plat)
			digest, ok := rel.Assets[expectedAsset]
			if !ok || digest == "" {
				t.Errorf("entry %s missing asset %s or has empty digest", rel.Version, expectedAsset)
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("entry %s asset %s digest does not have sha256: prefix: %s", rel.Version, expectedAsset, digest)
			}
		}

		if i > 0 {
			prev := parsedVersions.Protoc[i-1].Version
			curr := rel.Version
			if toolversions.Compare(prev, curr) <= 0 {
				t.Errorf("versions table is not sorted newest-first: %s is not newer than %s", prev, curr)
			}
		}
	}

	for _, plugin := range []string{"protoc-gen-go", "protoc-gen-go-grpc"} {
		ver, ok := PluginVersion(plugin)
		if !ok || ver == "" {
			t.Errorf("missing version for plugin %s", plugin)
		}
	}
}

// TestUpdateVersionsFile tests that update-versions rewrites versions.yaml against a mock GitHub API
// and follows Link headers across multiple pages.
func TestUpdateVersionsFile(t *testing.T) {
	page1Releases := []githubRelease{
		{
			TagName: "v37.0",
			Assets: []githubAsset{
				{Name: "protoc-37.0-linux-x86_64.zip", Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
				{Name: "protoc-37.0-linux-aarch_64.zip", Digest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
				{Name: "protoc-37.0-osx-x86_64.zip", Digest: "sha256:3333333333333333333333333333333333333333333333333333333333333333"},
				{Name: "protoc-37.0-osx-aarch_64.zip", Digest: "sha256:4444444444444444444444444444444444444444444444444444444444444444"},
				{Name: "protoc-37.0-win64.zip", Digest: "sha256:5555555555555555555555555555555555555555555555555555555555555555"},
			},
		},
	}
	page2Releases := []githubRelease{
		{
			TagName: "v36.9",
			Assets: []githubAsset{
				{Name: "protoc-36.9-linux-x86_64.zip", Digest: "sha256:6666666666666666666666666666666666666666666666666666666666666666"},
				{Name: "protoc-36.9-linux-aarch_64.zip", Digest: "sha256:7777777777777777777777777777777777777777777777777777777777777777"},
				{Name: "protoc-36.9-osx-x86_64.zip", Digest: "sha256:8888888888888888888888888888888888888888888888888888888888888888"},
				{Name: "protoc-36.9-osx-aarch_64.zip", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999"},
				{Name: "protoc-36.9-win64.zip", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			},
		},
	}

	var page1Requested, page2Requested bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			page2Requested = true
			_ = json.NewEncoder(w).Encode(page2Releases)
			return
		}
		page1Requested = true
		w.Header().Set("Link", fmt.Sprintf("<%s/repos/protocolbuffers/protobuf/releases?per_page=100&page=2>; rel=\"next\"", server.URL))
		_ = json.NewEncoder(w).Encode(page1Releases)
	}))
	defer server.Close()

	t.Setenv("PROTOC_GITHUB_API_BASE_URL", server.URL)

	tempRepo := t.TempDir()
	yamlDir := filepath.Join(tempRepo, "ap", "pkg", "tool", "protoc")
	if err := os.MkdirAll(yamlDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := scoped.WithHTTPClient(t.Context(), server.Client())
	err := UpdateVersionsFile(ctx, tempRepo)
	if err != nil {
		t.Fatalf("UpdateVersionsFile failed: %v", err)
	}

	if !page1Requested {
		t.Errorf("expected page 1 to be requested")
	}
	if !page2Requested {
		t.Errorf("expected page 2 to be requested via Link header")
	}

	written, err := os.ReadFile(filepath.Join(yamlDir, "versions.yaml"))
	if err != nil {
		t.Fatalf("failed to read written versions.yaml: %v", err)
	}

	var parsed versionsData
	if err := yaml.Unmarshal(written, &parsed); err != nil {
		t.Fatalf("failed to parse rewritten versions.yaml: %v", err)
	}

	if len(parsed.Protoc) != 2 {
		t.Fatalf("expected 2 protoc releases across both pages, got: %d", len(parsed.Protoc))
	}
	if parsed.Protoc[0].Version != "37.0" {
		t.Errorf("expected parsed.Protoc[0].Version to be 37.0, got: %s", parsed.Protoc[0].Version)
	}
	if parsed.Protoc[1].Version != "36.9" {
		t.Errorf("expected parsed.Protoc[1].Version to be 36.9, got: %s", parsed.Protoc[1].Version)
	}
	if parsed.Plugins["protoc-gen-go"] == "" {
		t.Errorf("expected protoc-gen-go plugin version, got empty")
	}
}

// TestNoGoGenerateDirective verifies that no //go:generate directive exists under
// ap/pkg/tool/protoc so that ap generate does not automatically run update-versions in CI.
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
