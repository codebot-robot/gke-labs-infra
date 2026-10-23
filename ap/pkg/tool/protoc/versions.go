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
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed versions.yaml
var embeddedVersionsYAML []byte

var supportedPlatforms = []string{
	"linux-x86_64",
	"linux-aarch_64",
	"osx-x86_64",
	"osx-aarch_64",
	"win64",
}

type versionsData struct {
	Protoc  []Release         `yaml:"protoc"`
	Plugins map[string]string `yaml:"plugins"`
}

// Release represents a known protoc release and its asset checksum digests.
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

// Latest returns the newest known protoc release from the table, or nil if empty.
func Latest() *Release {
	if len(parsedVersions.Protoc) == 0 {
		return nil
	}
	return &parsedVersions.Protoc[0]
}

// Lookup finds a protoc release by version string (with or without 'v' prefix).
func Lookup(version string) (*Release, bool) {
	norm := strings.TrimPrefix(version, "v")
	for i := range parsedVersions.Protoc {
		if parsedVersions.Protoc[i].Version == norm {
			return &parsedVersions.Protoc[i], true
		}
	}
	return nil, false
}

// PluginVersion returns the pinned version for a plugin from the table.
func PluginVersion(name string) (string, bool) {
	norm := name
	if !strings.HasPrefix(norm, "protoc-gen-") {
		norm = "protoc-gen-" + norm
	}
	v, ok := parsedVersions.Plugins[norm]
	return v, ok
}
