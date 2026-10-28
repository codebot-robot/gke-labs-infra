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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

const maxReleases = 5

type versionsData struct {
	Kubectl []toolversions.Release `yaml:"kubectl"`
}

// Update fetches the newest kubectl releases and digests from the Kubernetes release feed
// and updates ap/pkg/tool/kubectl/versions.yaml with the top 5 releases.
func Update(ctx context.Context, repoRoot string) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}

	feedURL := os.Getenv("KUBECTL_RELEASE_FEED_URL")
	if feedURL == "" {
		feedURL = "https://dl.k8s.io/release/stable.txt"
	}

	baseURL := os.Getenv("KUBECTL_DOWNLOAD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://dl.k8s.io/release"
	}

	latestTxt, err := fetchText(ctx, feedURL)
	if err != nil {
		return fmt.Errorf("failed to fetch kubectl release feed from %s: %w", feedURL, err)
	}
	latestVer := NormalizeTag(latestTxt)

	major, minor, err := parseMajorMinor(latestVer)
	if err != nil {
		return fmt.Errorf("failed to parse release version from %s: %w", feedURL, err)
	}

	versionList := []string{latestVer}
	for m := minor - 1; m >= minor-(maxReleases-1) && m >= 0; m-- {
		streamURL := fmt.Sprintf("%s/stable-%d.%d.txt", strings.TrimRight(baseURL, "/"), major, m)
		txt, err := fetchText(ctx, streamURL)
		if err != nil {
			klog.V(2).Infof("could not fetch minor stream from %s: %v", streamURL, err)
			break
		}
		ver := NormalizeTag(txt)
		if ver != "" {
			versionList = append(versionList, ver)
		}
	}

	var releases []toolversions.Release
	seenVersions := make(map[string]bool)

	for _, ver := range versionList {
		if seenVersions[ver] {
			continue
		}

		assetsMap := make(map[string]string)
		for _, plat := range supportedPlatforms {
			parts := strings.Split(plat, "-")
			if len(parts) != 2 {
				continue
			}
			goos, goarch := parts[0], parts[1]

			shaURL, err := ResolveKubectlSha256URL(baseURL, ver, goos, goarch)
			if err != nil {
				klog.V(2).Infof("failed to resolve sha URL for %s on %s: %v", ver, plat, err)
				break
			}

			shaText, err := fetchText(ctx, shaURL)
			if err != nil {
				klog.V(2).Infof("failed to fetch sha256 for %s on %s from %s: %v", ver, plat, shaURL, err)
				break
			}

			fields := strings.Fields(shaText)
			if len(fields) == 0 {
				klog.V(2).Infof("empty sha256 response for %s on %s from %s", ver, plat, shaURL)
				break
			}

			digest := fields[0]
			if !strings.HasPrefix(digest, "sha256:") {
				digest = "sha256:" + digest
			}
			assetsMap[plat] = digest
		}

		if len(assetsMap) == len(supportedPlatforms) {
			releases = append(releases, toolversions.Release{
				Version: ver,
				Assets:  assetsMap,
			})
			seenVersions[ver] = true
		}
	}

	if len(releases) == 0 {
		return fmt.Errorf("no kubectl releases found from release feed %s", feedURL)
	}

	toolversions.SortNewestFirst(releases)
	if len(releases) > maxReleases {
		releases = releases[:maxReleases]
	}

	data, err := yaml.Marshal(&versionsData{
		Kubectl: releases,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal kubectl versions data: %w", err)
	}

	targetPath := filepath.Join(repoRoot, "ap", "pkg", "tool", "kubectl", "versions.yaml")
	if err := fileutils.WriteFileAtomic(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	return nil
}

func parseMajorMinor(version string) (int, int, error) {
	norm := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if idx := strings.IndexAny(norm, "-+"); idx != -1 {
		norm = norm[:idx]
	}
	parts := strings.Split(norm, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid version string: %q", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major version in %q: %w", version, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minor version in %q: %w", version, err)
	}
	return major, minor, nil
}
