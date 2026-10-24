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

	toolversions "github.com/gke-labs/gke-labs-infra/ap/pkg/tool/versions"
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
	Protoc  []toolversions.Release `yaml:"protoc"`
	Plugins map[string]string      `yaml:"plugins"`
}

var parsedVersions = func() versionsData {
	protocRels, err := toolversions.Parse(embeddedVersionsYAML, "protoc")
	if err != nil {
		panic(fmt.Sprintf("failed to parse embedded versions.yaml: %v", err))
	}
	var raw struct {
		Plugins map[string]string `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(embeddedVersionsYAML, &raw); err != nil {
		panic(fmt.Sprintf("failed to parse embedded versions.yaml plugins: %v", err))
	}
	return versionsData{
		Protoc:  protocRels,
		Plugins: raw.Plugins,
	}
}()

// Latest returns the newest known protoc release from the table, or nil if empty.
func Latest() *toolversions.Release {
	return toolversions.Latest(parsedVersions.Protoc)
}

// Lookup finds a protoc release by version string (with or without 'v' prefix).
func Lookup(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedVersions.Protoc, version)
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
