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

package docker

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
	parsedDockerReleases = func() []toolversions.Release {
		rels, err := toolversions.Parse(embeddedVersionsYAML, "docker")
		if err != nil {
			panic(fmt.Sprintf("failed to parse embedded versions.yaml docker: %v", err))
		}
		return rels
	}()

	parsedBuildxReleases = func() []toolversions.Release {
		rels, err := toolversions.Parse(embeddedVersionsYAML, "buildx")
		if err != nil {
			panic(fmt.Sprintf("failed to parse embedded versions.yaml buildx: %v", err))
		}
		return rels
	}()
)

// LatestDocker returns the newest known docker release from the table, or nil if empty.
func LatestDocker() *toolversions.Release {
	return toolversions.Latest(parsedDockerReleases)
}

// LookupDocker finds a docker release by version string (with or without 'v' prefix).
func LookupDocker(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedDockerReleases, version)
}

// LatestBuildx returns the newest known buildx release from the table, or nil if empty.
func LatestBuildx() *toolversions.Release {
	return toolversions.Latest(parsedBuildxReleases)
}

// LookupBuildx finds a buildx release by version string (with or without 'v' prefix).
func LookupBuildx(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedBuildxReleases, version)
}
