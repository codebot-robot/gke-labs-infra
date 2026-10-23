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

package unused

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestConfig_Decode(t *testing.T) {
	data := []byte(`
mode: warn
skipGenerated: true
parameters:
  mode: warn
`)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Mode != "warn" {
		t.Errorf("got mode %q, want %q", cfg.Mode, "warn")
	}
	if cfg.SkipGenerated == nil || !*cfg.SkipGenerated {
		t.Errorf("got skipGenerated %v, want true", cfg.SkipGenerated)
	}
	if cfg.Parameters == nil || cfg.Parameters.Mode != "warn" {
		t.Errorf("got parameters.mode %v, want warn", cfg.Parameters)
	}
	flags := ExtraFlags(&cfg)
	if len(flags) != 1 || flags[0] != "-check-parameters=true" {
		t.Errorf("got flags %v, want [-check-parameters=true]", flags)
	}
}
