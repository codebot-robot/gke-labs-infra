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

var supportedAssets = []string{
	"kind-darwin-amd64",
	"kind-darwin-arm64",
	"kind-linux-amd64",
	"kind-linux-arm64",
	"kind-windows-amd64",
}

var parsedReleases = func() []toolversions.Release {
	rels, err := toolversions.Parse(embeddedVersionsYAML, "kind")
	if err != nil {
		panic(fmt.Sprintf("failed to parse embedded versions.yaml kind: %v", err))
	}
	return rels
}()

// Latest returns the newest known kind release from the table, or nil if empty.
func Latest() *toolversions.Release {
	return toolversions.Latest(parsedReleases)
}

// Lookup finds a kind release by version string (with or without 'v' prefix).
func Lookup(version string) (*toolversions.Release, bool) {
	return toolversions.Lookup(parsedReleases, version)
}
