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

package config

import (
	"reflect"
	"testing"
)

func TestSettingsDefaults(t *testing.T) {
	t.Run("nil means all defaults", func(t *testing.T) {
		var s *RepositorySettings
		if got, want := s.WithDefaults(), DefaultRepositorySettings(); !reflect.DeepEqual(got, want) {
			t.Errorf("WithDefaults() = %+v, want %+v", got, want)
		}
		if got := s.WithoutDefaults(); got != nil {
			t.Errorf("WithoutDefaults() on nil = %+v, want nil", got)
		}
	})

	t.Run("defaults strip to nil", func(t *testing.T) {
		def := DefaultRepositorySettings()
		if got := def.WithoutDefaults(); got != nil {
			t.Errorf("WithoutDefaults() = %+v, want nil", got)
		}
	})

	t.Run("non-defaults survive a round trip", func(t *testing.T) {
		in := &RepositorySettings{HasWiki: ptr(false), AllowAutoMerge: ptr(true), HasIssues: ptr(true)}
		stripped := in.WithoutDefaults()
		want := &RepositorySettings{HasWiki: ptr(false), AllowAutoMerge: ptr(true)}
		if !reflect.DeepEqual(stripped, want) {
			t.Errorf("WithoutDefaults() = %+v, want %+v", stripped, want)
		}
		full := stripped.WithDefaults()
		if *full.HasWiki || !*full.AllowAutoMerge || !*full.HasIssues || !*full.AllowSquashMerge {
			t.Errorf("WithDefaults() = %+v", full)
		}
	})
}
