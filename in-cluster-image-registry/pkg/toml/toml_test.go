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

package toml

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseTableHeader(t *testing.T) {
	tests := []struct {
		line      string
		wantParts []string
		wantOk    bool
	}{
		{
			line:      `[debug]`,
			wantParts: []string{"debug"},
			wantOk:    true,
		},
		{
			line:      `[plugins."io.containerd.grpc.v1.cri"]`,
			wantParts: []string{"plugins", "io.containerd.grpc.v1.cri"},
			wantOk:    true,
		},
		{
			line:      `[plugins.'io.containerd.grpc.v1.cri']`,
			wantParts: []string{"plugins", "io.containerd.grpc.v1.cri"},
			wantOk:    true,
		},
		{
			line:      `[ plugins . "io.containerd.grpc.v1.cri" . registry ]`,
			wantParts: []string{"plugins", "io.containerd.grpc.v1.cri", "registry"},
			wantOk:    true,
		},
		{
			line:      `[plugins.'io.containerd.cri.v1.images'.registry]`,
			wantParts: []string{"plugins", "io.containerd.cri.v1.images", "registry"},
			wantOk:    true,
		},
		{
			line:      `[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]`,
			wantParts: []string{"plugins", "io.containerd.grpc.v1.cri", "registry", "mirrors", "docker.io"},
			wantOk:    true,
		},
		{
			line:      `[plugins."io.containerd.grpc.v1.cri".registry] # some comment`,
			wantParts: []string{"plugins", "io.containerd.grpc.v1.cri", "registry"},
			wantOk:    true,
		},
		{
			line:      `# [plugins."io.containerd.grpc.v1.cri".registry]`,
			wantParts: nil,
			wantOk:    false,
		},
		{
			line:      `endpoint = ["https://mirror.gcr.io"]`,
			wantParts: nil,
			wantOk:    false,
		},
		{
			line:      `[[plugins]]`,
			wantParts: nil,
			wantOk:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			gotParts, gotOk := ParseTableHeader(tc.line)
			if gotOk != tc.wantOk {
				t.Errorf("ParseTableHeader(%q) gotOk=%v, wantOk=%v", tc.line, gotOk, tc.wantOk)
			}
			if diff := cmp.Diff(tc.wantParts, gotParts); diff != "" {
				t.Errorf("ParseTableHeader(%q) parts mismatch (-want +got):\n%s", tc.line, diff)
			}
		})
	}
}

func TestParseEndpoints(t *testing.T) {
	lines := []string{
		`  endpoint = ["https://mirror.gcr.io","https://registry-1.docker.io"]`,
	}
	got := ParseEndpoints(lines)
	want := []string{"https://mirror.gcr.io", "https://registry-1.docker.io"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseEndpoints mismatch (-want +got):\n%s", diff)
	}
}
