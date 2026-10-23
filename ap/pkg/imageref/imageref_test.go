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

package imageref

import (
	"reflect"
	"testing"
)

func TestParseAndString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      Reference
		wantName  string
		expectErr bool
	}{
		{
			name:     "golang",
			input:    "golang",
			want:     Reference{Repository: "golang"},
			wantName: "golang",
		},
		{
			name:     "golang:1.25",
			input:    "golang:1.25",
			want:     Reference{Repository: "golang", Tag: "1.25"},
			wantName: "golang",
		},
		{
			name:  "docker.io/library/golang:1.25-alpine",
			input: "docker.io/library/golang:1.25-alpine",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/golang",
				Tag:        "1.25-alpine",
			},
			wantName: "golang",
		},
		{
			name:  "localhost:5000/foo:bar",
			input: "localhost:5000/foo:bar",
			want: Reference{
				Registry:   "localhost:5000",
				Repository: "foo",
				Tag:        "bar",
			},
			wantName: "foo",
		},
		{
			name:  "gcr.io/distroless/static:nonroot",
			input: "gcr.io/distroless/static:nonroot",
			want: Reference{
				Registry:   "gcr.io",
				Repository: "distroless/static",
				Tag:        "nonroot",
			},
			wantName: "static",
		},
		{
			name:  "golang@sha256:abc",
			input: "golang@sha256:abc",
			want: Reference{
				Repository: "golang",
				Digest:     "sha256:abc",
			},
			wantName: "golang",
		},
		{
			name:  "golang:1.25@sha256:abc",
			input: "golang:1.25@sha256:abc",
			want: Reference{
				Repository: "golang",
				Tag:        "1.25",
				Digest:     "sha256:abc",
			},
			wantName: "golang",
		},
		{
			name:      "${IMAGE}",
			input:     "${IMAGE}",
			expectErr: true,
		},
		{
			name:      "$IMAGE",
			input:     "$IMAGE",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := Parse(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(ref, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.input, ref, tt.want)
			}
			if gotName := ref.Name(); gotName != tt.wantName {
				t.Errorf("ref.Name() = %q, want %q", gotName, tt.wantName)
			}
			if gotStr := ref.String(); gotStr != tt.input {
				t.Errorf("ref.String() = %q, want %q (round trip failed)", gotStr, tt.input)
			}
		})
	}
}
