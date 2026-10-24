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
	"strings"

	"gopkg.in/yaml.v3"
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

type versionsData struct {
	Docker []Release `yaml:"docker"`
	Buildx []Release `yaml:"buildx"`
}

// Release represents a known tool release and its per-platform checksum digests.
type Release struct {
	Version string            `yaml:"version"`
	Assets  map[string]string `yaml:"assets"`
}

var parsedVersions = func() versionsData {
	var v versionsData
	if err := yaml.Unmarshal(embeddedVersionsYAML, &v); err != nil {
		panic(fmt.Sprintf("failed to parse embedded versions.yaml: %v", err))
	}
	return v
}()

// LatestDocker returns the newest known docker release from the table, or nil if empty.
func LatestDocker() *Release {
	if len(parsedVersions.Docker) == 0 {
		return nil
	}
	return &parsedVersions.Docker[0]
}

// LookupDocker finds a docker release by version string (with or without 'v' prefix).
func LookupDocker(version string) (*Release, bool) {
	norm := strings.TrimPrefix(strings.TrimSpace(version), "v")
	for i := range parsedVersions.Docker {
		if parsedVersions.Docker[i].Version == norm {
			return &parsedVersions.Docker[i], true
		}
	}
	return nil, false
}

// LatestBuildx returns the newest known buildx release from the table, or nil if empty.
func LatestBuildx() *Release {
	if len(parsedVersions.Buildx) == 0 {
		return nil
	}
	return &parsedVersions.Buildx[0]
}

// LookupBuildx finds a buildx release by version string (with or without 'v' prefix).
func LookupBuildx(version string) (*Release, bool) {
	norm := strings.TrimPrefix(strings.TrimSpace(version), "v")
	for i := range parsedVersions.Buildx {
		if strings.TrimPrefix(parsedVersions.Buildx[i].Version, "v") == norm {
			return &parsedVersions.Buildx[i], true
		}
	}
	return nil, false
}
