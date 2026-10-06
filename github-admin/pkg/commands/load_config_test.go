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

package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
)

func TestLoadConfigs(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		want    []config.RepositoryConfig
		wantErr bool
	}{
		{
			name: "List format (Not supported anymore as per PR feedback)",
			content: `- owner: org1
  name: repo1
- owner: org2
  name: repo2
`,
			// It will try to parse list as single object -> fail?
			// Actually, mapstructure/yaml might partial match or fail.
			// Since we removed list support, this test case expectation should change or be removed.
			// If we parse a list as a struct, it usually errors because [] != struct.
			wantErr: true,
		},
		{
			name: "Multi-doc format",
			content: `owner: org1
name: repo1
---
owner: org2
name: repo2
`,
			want: []config.RepositoryConfig{
				{Owner: "org1", Name: "repo1"},
				{Owner: "org2", Name: "repo2"},
			},
		},
		{
			name:    "Invalid YAML",
			content: `invalid: [`,
			wantErr: true,
		},
		{
			name: "With Merge Settings",
			content: `owner: org1
name: repo1
settings:
  mergeCommitTitle: PR_TITLE
  mergeCommitMessage: PR_BODY
`,
			want: []config.RepositoryConfig{
				{
					Owner: "org1",
					Name:  "repo1",
					Settings: &config.RepositorySettings{
						MergeCommitTitle:   stringPtr("PR_TITLE"),
						MergeCommitMessage: stringPtr("PR_BODY"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tempDir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			got, err := LoadConfigs(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfigs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadConfigs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigsDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"b.yaml":       "owner: org1\nname: repo-b\n",
		"a.yaml":       "owner: org1\nname: repo-a\ndefaultRulesets:\n  - require-pr-reviews\n",
		"nested/c.yml": "owner: org2\nname: repo-c\n",
		"README.md":    "not yaml",
		"notes.txt":    "owner: ignored\nname: ignored\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := LoadConfigs(dir)
	if err != nil {
		t.Fatalf("LoadConfigs() error = %v", err)
	}
	want := []config.RepositoryConfig{
		{Owner: "org1", Name: "repo-a", DefaultRulesets: []string{"require-pr-reviews"}},
		{Owner: "org1", Name: "repo-b"},
		{Owner: "org2", Name: "repo-c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadConfigs() = %+v, want %+v", got, want)
	}
}

func stringPtr(s string) *string {
	return &s
}
