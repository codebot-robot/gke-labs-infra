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

package testonlyimports

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestDecodeSettings(t *testing.T) {
	yamlContent := `
mode: warn
packages:
  - example.com/extra
skipGenerated: true
skipTests: true
`
	var s Settings
	if err := yaml.Unmarshal([]byte(yamlContent), &s); err != nil {
		t.Fatalf("failed to unmarshal Settings: %v", err)
	}
	if s.Mode != "warn" {
		t.Errorf("expected Mode warn, got %q", s.Mode)
	}
	if len(s.Packages) != 1 || s.Packages[0] != "example.com/extra" {
		t.Errorf("expected Packages [example.com/extra], got %v", s.Packages)
	}
	if s.SkipGeneratedOverride() == nil || *s.SkipGeneratedOverride() != true {
		t.Errorf("expected SkipGeneratedOverride true")
	}
	if s.SkipTestsOverride() == nil || *s.SkipTestsOverride() != true {
		t.Errorf("expected SkipTestsOverride true")
	}
}
