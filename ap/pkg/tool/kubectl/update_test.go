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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
)

func TestUpdate(t *testing.T) {
	dummyHash := "c04487523e7aa7d1493f7808174c899db93d7a9cf57531420cd76b9bd415f8f4"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable.txt":
			_, _ = fmt.Fprintln(w, "v1.37.2")
		case "/stable-1.36.txt":
			_, _ = fmt.Fprintln(w, "v1.36.6")
		case "/stable-1.35.txt":
			_, _ = fmt.Fprintln(w, "v1.35.10")
		default:
			if strings.HasSuffix(r.URL.Path, ".sha256") {
				_, _ = fmt.Fprintln(w, dummyHash)
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempRepo := t.TempDir()
	targetDir := filepath.Join(tempRepo, "ap", "pkg", "tool", "kubectl")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KUBECTL_RELEASE_FEED_URL", server.URL+"/stable.txt")
	t.Setenv("KUBECTL_DOWNLOAD_BASE_URL", server.URL)

	if err := Update(t.Context(), tempRepo); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	yamlPath := filepath.Join(targetDir, "versions.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("reading generated versions.yaml: %v", err)
	}

	releases, err := toolversions.Parse(data, "kubectl")
	if err != nil {
		t.Fatalf("parsing generated versions.yaml: %v", err)
	}

	if len(releases) < 3 {
		t.Fatalf("expected at least 3 releases, got %d", len(releases))
	}

	if releases[0].Version != "v1.37.2" {
		t.Errorf("expected newest release v1.37.2, got %s", releases[0].Version)
	}

	for _, plat := range supportedPlatforms {
		digest, ok := releases[0].Assets[plat]
		if !ok {
			t.Errorf("missing asset for %s in %s", plat, releases[0].Version)
		}
		if digest != "sha256:"+dummyHash {
			t.Errorf("expected digest sha256:%s, got %s", dummyHash, digest)
		}
	}
}
