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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// UpdateVersionsFile fetches protoc releases from the GitHub API and plugin versions
// using 'go list -m -versions', then rewrites ap/pkg/tool/protoc/versions.yaml atomically.
func UpdateVersionsFile(ctx context.Context, repoRoot string, client *http.Client) error {
	if client == nil {
		client = getHTTPClient()
	}

	apiBase := os.Getenv("PROTOC_GITHUB_API_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

	var releases []Release
	seenVersions := make(map[string]bool)

	for page := 1; ; page++ {
		apiURL := fmt.Sprintf("%s/repos/protocolbuffers/protobuf/releases?per_page=100&page=%d", strings.TrimRight(apiBase, "/"), page)
		req, err := newGitHubRequest(ctx, apiURL)
		if err != nil {
			return fmt.Errorf("failed to create request for %s: %w", apiURL, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to query GitHub releases API: %w", err)
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				klog.V(2).Infof("failed to close response body: %v", closeErr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub releases API returned status %s for %s", resp.Status, apiURL)
		}

		var pageReleases []githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&pageReleases); err != nil {
			return fmt.Errorf("failed to decode GitHub releases: %w", err)
		}

		if len(pageReleases) == 0 {
			break
		}

		for _, rel := range pageReleases {
			if rel.Draft || rel.Prerelease {
				continue
			}
			ver := strings.TrimPrefix(rel.TagName, "v")
			if seenVersions[ver] {
				continue
			}

			assetsMap := make(map[string]string)
			for _, plat := range supportedPlatforms {
				expectedAsset := fmt.Sprintf("protoc-%s-%s.zip", ver, plat)
				asset, found := findAsset(rel.Assets, expectedAsset)
				if !found || asset.Digest == "" {
					break
				}
				assetsMap[expectedAsset] = asset.Digest
			}

			if len(assetsMap) == len(supportedPlatforms) {
				releases = append(releases, Release{
					Version: ver,
					Assets:  assetsMap,
				})
				seenVersions[ver] = true
			}
		}

		if len(pageReleases) < 100 {
			break
		}
	}

	// Sort releases newest first
	sort.Slice(releases, func(i, j int) bool {
		return compareVersions(releases[i].Version, releases[j].Version) > 0
	})

	// Resolve latest plugin versions via go list
	plugins := make(map[string]string)
	pluginModules := map[string]string{
		"protoc-gen-go":      "google.golang.org/protobuf",
		"protoc-gen-go-grpc": "google.golang.org/grpc/cmd/protoc-gen-go-grpc",
	}
	pluginOrder := []string{"protoc-gen-go", "protoc-gen-go-grpc"}
	for _, pluginName := range pluginOrder {
		mod := pluginModules[pluginName]
		if mod == "" {
			continue
		}
		cmd := exec.CommandContext(ctx, "go", "list", "-m", "-versions", mod)
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=auto")
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to list versions for plugin %s (%s): %w", pluginName, mod, err)
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return fmt.Errorf("no versions returned for plugin %s", pluginName)
		}

		var newest string
		for i := len(fields) - 1; i >= 1; i-- {
			v := fields[i]
			if !strings.Contains(v, "-") {
				newest = v
				break
			}
		}
		if newest == "" {
			newest = fields[len(fields)-1]
		}
		plugins[pluginName] = newest
	}

	data, err := yaml.Marshal(&versionsData{
		Protoc:  releases,
		Plugins: plugins,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal versions data: %w", err)
	}

	targetPath := filepath.Join(repoRoot, "ap", "pkg", "tool", "protoc", "versions.yaml")
	if err := fileutils.WriteFileAtomic(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	return nil
}

func compareVersions(v1, v2 string) int {
	parts1 := parseVersionParts(v1)
	parts2 := parseVersionParts(v2)

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1 := 0
		if i < len(parts1) {
			p1 = parts1[i]
		}
		p2 := 0
		if i < len(parts2) {
			p2 = parts2[i]
		}
		if p1 != p2 {
			if p1 > p2 {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	rawParts := strings.Split(v, ".")
	var parts []int
	for _, p := range rawParts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		parts = append(parts, n)
	}
	return parts
}
