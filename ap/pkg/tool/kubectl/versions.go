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
	_ "embed"
	"fmt"
	"strings"

	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
)

//go:embed versions.yaml
var embeddedVersionsYAML []byte

var supportedPlatforms = []string{
	"linux-amd64",
	"linux-arm64",
	"darwin-amd64",
	"darwin-arm64",
	"windows-amd64",
}

var (
	parsedKubectlReleases = func() []toolversions.Release {
		rels, err := toolversions.Parse(embeddedVersionsYAML, "kubectl")
		if err != nil {
			panic(fmt.Sprintf("failed to parse embedded versions.yaml kubectl: %v", err))
		}
		return rels
	}()
)

// LatestKubectl returns the newest known kubectl release from the table, or nil if empty.
func LatestKubectl() *toolversions.Release {
	return toolversions.Latest(parsedKubectlReleases)
}

// LookupKubectl finds a kubectl release by version string (with or without 'v' prefix).
func LookupKubectl(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedKubectlReleases, version)
}

// NormalizeTag ensures that the version string has a leading "v".
func NormalizeTag(version string) string {
	v := strings.TrimSpace(version)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// ResolvePlatform maps runtime.GOOS and runtime.GOARCH to the supported platform identifier.
func ResolvePlatform(goos, goarch string) (string, error) {
	switch goos {
	case "linux":
		switch goarch {
		case "amd64", "arm64":
			return goos + "-" + goarch, nil
		default:
			return "", fmt.Errorf("unsupported linux architecture: %s", goarch)
		}
	case "darwin":
		switch goarch {
		case "amd64", "arm64":
			return goos + "-" + goarch, nil
		default:
			return "", fmt.Errorf("unsupported darwin architecture: %s", goarch)
		}
	case "windows":
		switch goarch {
		case "amd64":
			return goos + "-" + goarch, nil
		default:
			return "", fmt.Errorf("unsupported windows architecture: %s", goarch)
		}
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
}

// ResolveKubectlDigest looks up the SHA-256 digest for the given platform in a release.
func ResolveKubectlDigest(rel *toolversions.Release, goos, goarch string) (string, error) {
	plat, err := ResolvePlatform(goos, goarch)
	if err != nil {
		return "", err
	}
	if rel.Assets != nil {
		if d, ok := rel.Assets[plat]; ok && d != "" {
			return d, nil
		}
		if d, ok := rel.Assets["kubectl-"+plat]; ok && d != "" {
			return d, nil
		}
	}
	return "", fmt.Errorf("kubectl %s has no asset for platform %s in versions.yaml", rel.Version, plat)
}

// ResolveKubectlDownloadURL computes the download URL for kubectl release binaries.
func ResolveKubectlDownloadURL(baseURL, tag, goos, goarch string) (string, error) {
	normTag := NormalizeTag(tag)
	if _, err := ResolvePlatform(goos, goarch); err != nil {
		return "", err
	}
	binName := "kubectl"
	if goos == "windows" {
		binName = "kubectl.exe"
	}
	if baseURL == "" {
		baseURL = "https://dl.k8s.io/release"
	}
	return fmt.Sprintf("%s/%s/bin/%s/%s/%s", strings.TrimRight(baseURL, "/"), normTag, goos, goarch, binName), nil
}

// ResolveKubectlSha256URL computes the URL for the published .sha256 digest file on dl.k8s.io.
func ResolveKubectlSha256URL(baseURL, tag, goos, goarch string) (string, error) {
	downloadURL, err := ResolveKubectlDownloadURL(baseURL, tag, goos, goarch)
	if err != nil {
		return "", err
	}
	return downloadURL + ".sha256", nil
}
