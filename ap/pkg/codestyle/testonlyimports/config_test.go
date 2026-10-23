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

func TestConfig_Decode(t *testing.T) {
	data := []byte(`
mode: warn
packages:
  - github.com/google/go-cmp
  - example.com/pkg
`)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Mode != "warn" {
		t.Errorf("got mode %q, want %q", cfg.Mode, "warn")
	}
	if len(cfg.Packages) != 2 || cfg.Packages[0] != "github.com/google/go-cmp" || cfg.Packages[1] != "example.com/pkg" {
		t.Errorf("unexpected packages: %v", cfg.Packages)
	}
	flags := ExtraFlags(&cfg)
	if len(flags) != 2 || flags[0] != "-package=github.com/google/go-cmp" || flags[1] != "-package=example.com/pkg" {
		t.Errorf("unexpected flags: %v", flags)
	}
}
