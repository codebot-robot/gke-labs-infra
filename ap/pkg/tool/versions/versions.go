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

package versions

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Release represents a known tool release and its per-platform asset digests.
type Release struct {
	Version string            `yaml:"version"`
	Assets  map[string]string `yaml:"assets"`
}

// Parse parses and strictly decodes the given section from YAML data into []Release.
func Parse(data []byte, section string) ([]Release, error) {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	secNode, ok := raw[section]
	if !ok {
		return nil, fmt.Errorf("section %q not found in versions data", section)
	}

	buf, err := yaml.Marshal(&secNode)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal section %q: %w", section, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(buf))
	dec.KnownFields(true)
	var releases []Release
	if err := dec.Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode releases for section %q: %w", section, err)
	}

	return releases, nil
}

// Latest returns the newest known release from the slice, or nil if empty.
func Latest(releases []Release) *Release {
	if len(releases) == 0 {
		return nil
	}
	return &releases[0]
}

// Lookup finds a release by version string (with or without 'v' prefix).
func Lookup(releases []Release, version string) (*Release, bool) {
	norm := strings.TrimPrefix(strings.TrimSpace(version), "v")
	for i := range releases {
		if strings.TrimPrefix(releases[i].Version, "v") == norm {
			return &releases[i], true
		}
	}
	return nil, false
}
