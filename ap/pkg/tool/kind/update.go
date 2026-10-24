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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/fetch"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/github"
	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

const maxReleases = 5

type versionsData struct {
	Kind []toolversions.Release `yaml:"kind"`
}

// Update fetches kind releases from the GitHub API and rewrites ap/pkg/tool/kind/versions.yaml atomically.
func Update(ctx context.Context, repoRoot string) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	apiBase := os.Getenv("KIND_GITHUB_API_BASE_URL")
	if apiBase == "" {
		apiBase = os.Getenv("GITHUB_API_BASE_URL")
	}
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

	rawReleases, err := github.ListReleases(ctx, apiBase, "kubernetes-sigs/kind", maxReleases*4)
	if err != nil {
		return err
	}

	var releases []toolversions.Release
	seenVersions := make(map[string]bool)

	for _, rel := range rawReleases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		ver := rel.TagName
		if !strings.HasPrefix(ver, "v") {
			ver = "v" + ver
		}
		if seenVersions[ver] {
			continue
		}

		assetsMap := make(map[string]string)
		for _, assetName := range supportedAssets {
			asset, found := github.FindAsset(rel.Assets, assetName)
			if !found {
				break
			}
			if asset.Digest != "" {
				assetsMap[assetName] = asset.Digest
				continue
			}

			// Check for <assetName>.sha256sum
			shaAsset, shaFound := github.FindAsset(rel.Assets, assetName+".sha256sum")
			if shaFound && shaAsset.BrowserDownloadURL != "" {
				content, err := github.GetText(ctx, shaAsset.BrowserDownloadURL)
				if err != nil {
					klog.V(2).Infof("failed to fetch kind checksum asset %s: %v", shaAsset.BrowserDownloadURL, err)
				} else {
					fields := strings.Fields(content)
					if len(fields) >= 1 && len(fields[0]) == 64 {
						assetsMap[assetName] = "sha256:" + fields[0]
						continue
					}
				}
			}

			// Fallback: compute digest via fetch.Digest
			if asset.BrowserDownloadURL != "" {
				d, err := fetch.Digest(ctx, asset.BrowserDownloadURL)
				if err != nil {
					klog.V(2).Infof("failed computing digest for %s: %v", asset.BrowserDownloadURL, err)
				} else if d != "" {
					assetsMap[assetName] = d
				}
			}
		}

		if len(assetsMap) == len(supportedAssets) {
			releases = append(releases, toolversions.Release{
				Version: ver,
				Assets:  assetsMap,
			})
			seenVersions[ver] = true
			if len(releases) == maxReleases {
				break
			}
		}
	}

	toolversions.SortNewestFirst(releases)
	if len(releases) > maxReleases {
		releases = releases[:maxReleases]
	}

	data, err := yaml.Marshal(&versionsData{
		Kind: releases,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal versions data: %w", err)
	}

	targetPath := filepath.Join(repoRoot, "ap", "pkg", "tool", "kind", "versions.yaml")
	if err := fileutils.WriteFileAtomic(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	return nil
}
