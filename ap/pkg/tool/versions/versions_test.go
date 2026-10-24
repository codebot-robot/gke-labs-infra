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
	"testing"
)

func TestParse_Success(t *testing.T) {
	data := []byte(`
tools:
  - version: "2.0.0"
    assets:
      linux-amd64: sha256:1111111111111111111111111111111111111111111111111111111111111111
  - version: "1.0.0"
    assets:
      linux-amd64: sha256:2222222222222222222222222222222222222222222222222222222222222222
`)

	rels, err := Parse(data, "tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(rels))
	}

	latest := Latest(rels)
	if latest == nil || latest.Version != "2.0.0" {
		t.Fatalf("expected latest version 2.0.0, got %v", latest)
	}

	found, ok := Lookup(rels, "1.0.0")
	if !ok || found.Version != "1.0.0" {
		t.Errorf("expected to find 1.0.0, got %v", found)
	}

	foundV, okV := Lookup(rels, "v1.0.0")
	if !okV || foundV.Version != "1.0.0" {
		t.Errorf("expected to find v1.0.0, got %v", foundV)
	}

	_, okNone := Lookup(rels, "9.9.9")
	if okNone {
		t.Error("expected Lookup to return false for nonexistent version")
	}
}

func TestParse_MissingSection(t *testing.T) {
	data := []byte(`
other:
  - version: "1.0.0"
`)

	_, err := Parse(data, "tools")
	if err == nil {
		t.Fatal("expected error for missing section, got nil")
	}
}

func TestParse_StrictRejectsUnknownFields(t *testing.T) {
	data := []byte(`
tools:
  - version: "1.0.0"
    unknown_field: "bad"
    assets:
      linux-amd64: sha256:1111111111111111111111111111111111111111111111111111111111111111
`)

	_, err := Parse(data, "tools")
	if err == nil {
		t.Fatal("expected strict unmarshal to reject unknown_field, got nil")
	}
}

func TestLatest_Empty(t *testing.T) {
	if got := Latest(nil); got != nil {
		t.Errorf("expected nil for empty slice, got %v", got)
	}
}
