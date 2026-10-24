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
	_ "embed"
	"fmt"

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
	parsedKindReleases = func() []toolversions.Release {
		rels, err := toolversions.Parse(embeddedVersionsYAML, "kind")
		if err != nil {
			panic(fmt.Sprintf("failed to parse embedded versions.yaml kind: %v", err))
		}
		return rels
	}()
)

// LatestKind returns the newest known kind release from the table, or nil if empty.
func LatestKind() *toolversions.Release {
	return toolversions.Latest(parsedKindReleases)
}

// LookupKind finds a kind release by version string (with or without 'v' prefix).
func LookupKind(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedKindReleases, version)
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

// ResolveKindAsset returns the GitHub release asset name for the given target OS and architecture.
func ResolveKindAsset(goos, goarch string) (string, error) {
	plat, err := ResolvePlatform(goos, goarch)
	if err != nil {
		return "", err
	}
	return "kind-" + plat, nil
}
