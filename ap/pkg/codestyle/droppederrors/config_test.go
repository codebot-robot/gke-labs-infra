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

package droppederrors

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestConfig_Decode(t *testing.T) {
	data := []byte(`
mode: error
exclude:
  - fmt.Println
baseline: .ap/baseline.txt
useDefaultExcludes: true
goos:
  - linux
`)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Mode != "error" {
		t.Errorf("got mode %q, want %q", cfg.Mode, "error")
	}
	if len(cfg.Exclude) != 1 || cfg.Exclude[0] != "fmt.Println" {
		t.Errorf("unexpected exclude: %v", cfg.Exclude)
	}
	if cfg.Baseline != ".ap/baseline.txt" {
		t.Errorf("got baseline %q, want .ap/baseline.txt", cfg.Baseline)
	}
	if cfg.UseDefaultExcludes == nil || !*cfg.UseDefaultExcludes {
		t.Errorf("got useDefaultExcludes %v, want true", cfg.UseDefaultExcludes)
	}
	if len(cfg.GOOS) != 1 || cfg.GOOS[0] != "linux" {
		t.Errorf("unexpected GOOS: %v", cfg.GOOS)
	}
}
