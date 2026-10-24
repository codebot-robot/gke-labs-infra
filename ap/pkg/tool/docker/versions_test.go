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
	"strings"
	"testing"
)

func TestVersionsYAML_Structure(t *testing.T) {
	if len(parsedVersions.Docker) == 0 {
		t.Fatal("no docker releases parsed from versions.yaml")
	}
	if len(parsedVersions.Buildx) == 0 {
		t.Fatal("no buildx releases parsed from versions.yaml")
	}

	for _, rel := range parsedVersions.Docker {
		if rel.Version == "" {
			t.Errorf("empty docker version in release")
		}
		for _, plat := range supportedPlatforms {
			digest, ok := rel.Assets[plat]
			if !ok || digest == "" {
				t.Errorf("docker release %s missing asset for platform %s", rel.Version, plat)
				continue
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("docker release %s platform %s digest does not have sha256: prefix: %s", rel.Version, plat, digest)
			}
			if len(digest) != len("sha256:")+64 {
				t.Errorf("docker release %s platform %s digest length is invalid: %s", rel.Version, plat, digest)
			}
		}
	}

	for _, rel := range parsedVersions.Buildx {
		if rel.Version == "" {
			t.Errorf("empty buildx version in release")
		}
		for _, plat := range supportedPlatforms {
			digest, ok := rel.Assets[plat]
			if !ok || digest == "" {
				t.Errorf("buildx release %s missing asset for platform %s", rel.Version, plat)
				continue
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("buildx release %s platform %s digest does not have sha256: prefix: %s", rel.Version, plat, digest)
			}
			if len(digest) != len("sha256:")+64 {
				t.Errorf("buildx release %s platform %s digest length is invalid: %s", rel.Version, plat, digest)
			}
		}
	}
}

func TestLookupDocker(t *testing.T) {
	rel, ok := LookupDocker("28.0.1")
	if !ok || rel == nil {
		t.Fatal("expected to find docker 28.0.1")
	}
	if rel.Version != "28.0.1" {
		t.Errorf("expected version 28.0.1, got %s", rel.Version)
	}

	relV, okV := LookupDocker("v28.0.1")
	if !okV || relV == nil {
		t.Fatal("expected to find docker v28.0.1")
	}

	_, okNone := LookupDocker("0.0.0-nonexistent")
	if okNone {
		t.Error("expected LookupDocker for nonexistent version to return false")
	}
}

func TestLookupBuildx(t *testing.T) {
	rel, ok := LookupBuildx("0.21.1")
	if !ok || rel == nil {
		t.Fatal("expected to find buildx 0.21.1")
	}

	relV, okV := LookupBuildx("v0.21.1")
	if !okV || relV == nil {
		t.Fatal("expected to find buildx v0.21.1")
	}

	_, okNone := LookupBuildx("v0.0.0-nonexistent")
	if okNone {
		t.Error("expected LookupBuildx for nonexistent version to return false")
	}
}
