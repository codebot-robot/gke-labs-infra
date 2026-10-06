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
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
)

const requirePRReviewsYAML = `name: require-pr-reviews
target: branch
enforcement: active
conditions:
  refName:
    include: ["~DEFAULT_BRANCH"]
bypassActors:
  - actorId: 5
    actorType: RepositoryRole
    bypassMode: always
rules:
  deletion: true
  nonFastForward: true
  pullRequest:
    requiredApprovingReviewCount: 1
    allowedMergeMethods: [merge, squash, rebase]
`

func writeDefaults(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadDefaultRulesets(t *testing.T) {
	t.Run("missing directory is empty", func(t *testing.T) {
		got, err := LoadDefaultRulesets(filepath.Join(t.TempDir(), "nope"))
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want empty, nil", got, err)
		}
	})

	t.Run("loads by file name", func(t *testing.T) {
		dir := writeDefaults(t, map[string]string{
			"require-pr-reviews.yaml": requirePRReviewsYAML,
			"README.md":               "ignored",
		})
		got, err := LoadDefaultRulesets(dir)
		if err != nil {
			t.Fatal(err)
		}
		rs, ok := got["require-pr-reviews"]
		if len(got) != 1 || !ok {
			t.Fatalf("got %v", got)
		}
		if rs.Rules == nil || rs.Rules.PullRequest == nil || rs.Rules.PullRequest.RequiredApprovingReviewCount != 1 {
			t.Errorf("unexpected ruleset: %+v", rs)
		}
	})

	t.Run("name must match file name", func(t *testing.T) {
		dir := writeDefaults(t, map[string]string{"other.yaml": requirePRReviewsYAML})
		_, err := LoadDefaultRulesets(dir)
		if err == nil || !strings.Contains(err.Error(), "must match the file name") {
			t.Errorf("expected name mismatch error, got %v", err)
		}
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		dir := writeDefaults(t, map[string]string{"x.yaml": "name: x\nbogus: true\n"})
		if _, err := LoadDefaultRulesets(dir); err == nil {
			t.Error("expected error for unknown field")
		}
	})
}

func TestResolveRulesets(t *testing.T) {
	defaults := map[string]*config.RepositoryRuleset{
		"require-pr-reviews": {Name: "require-pr-reviews"},
		"merge-queue":        {Name: "merge-queue"},
	}

	tests := []struct {
		name      string
		cfg       config.RepositoryConfig
		wantNames []string
		wantErr   string
	}{
		{
			name: "defaults then custom",
			cfg: config.RepositoryConfig{
				DefaultRulesets: []string{"merge-queue", "require-pr-reviews"},
				CustomRulesets:  []*config.RepositoryRuleset{{Name: "checks"}},
			},
			wantNames: []string{"merge-queue", "require-pr-reviews", "checks"},
		},
		{
			name:    "unknown default",
			cfg:     config.RepositoryConfig{Name: "r", DefaultRulesets: []string{"nope"}},
			wantErr: "unknown default ruleset",
		},
		{
			name:    "duplicate default",
			cfg:     config.RepositoryConfig{Name: "r", DefaultRulesets: []string{"merge-queue", "merge-queue"}},
			wantErr: "more than once",
		},
		{
			name: "custom shadows default",
			cfg: config.RepositoryConfig{
				Name:            "r",
				DefaultRulesets: []string{"require-pr-reviews"},
				CustomRulesets:  []*config.RepositoryRuleset{{Name: "require-pr-reviews"}},
			},
			wantErr: "both a default and a custom",
		},
		{
			name:    "custom without name",
			cfg:     config.RepositoryConfig{Name: "r", CustomRulesets: []*config.RepositoryRuleset{{}}},
			wantErr: "no name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRulesets(&tt.cfg, defaults)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got err %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, rs := range got {
				names = append(names, rs.Name)
			}
			if !reflect.DeepEqual(names, tt.wantNames) {
				t.Errorf("got %v, want %v", names, tt.wantNames)
			}
		})
	}
}

func TestClassifyRulesets(t *testing.T) {
	dir := writeDefaults(t, map[string]string{"require-pr-reviews.yaml": requirePRReviewsYAML})
	defaults, err := LoadDefaultRulesets(dir)
	if err != nil {
		t.Fatal(err)
	}

	// As mapRuleset would produce it from the API: same content, but with
	// empty rather than nil slices where GitHub returns [].
	match := &config.RepositoryRuleset{
		Name: "require-pr-reviews", Target: "branch", Enforcement: "active",
		Conditions: &config.RulesetConditions{
			RefName: &config.RefNameCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}},
		},
		BypassActors: []config.BypassActor{{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"}},
		Rules: &config.RulesetRules{
			Deletion: true, NonFastForward: true,
			PullRequest: &config.PullRequestRule{
				RequiredApprovingReviewCount: 1,
				AllowedMergeMethods:          []string{"merge", "squash", "rebase"},
			},
		},
	}
	// Same name, but two approvals required.
	differs := &config.RepositoryRuleset{}
	*differs = *match
	differs.Rules = &config.RulesetRules{
		Deletion: true, NonFastForward: true,
		PullRequest: &config.PullRequestRule{RequiredApprovingReviewCount: 2, AllowedMergeMethods: []string{"merge", "squash", "rebase"}},
	}
	other := &config.RepositoryRuleset{Name: "something-else", Enforcement: "active"}

	gotDefaults, gotCustom := classifyRulesets([]*config.RepositoryRuleset{other, match}, defaults)
	if !reflect.DeepEqual(gotDefaults, []string{"require-pr-reviews"}) {
		t.Errorf("defaults = %v", gotDefaults)
	}
	if len(gotCustom) != 1 || gotCustom[0] != other {
		t.Errorf("custom = %+v", gotCustom)
	}

	gotDefaults, gotCustom = classifyRulesets([]*config.RepositoryRuleset{differs}, defaults)
	if len(gotDefaults) != 0 || len(gotCustom) != 1 {
		t.Errorf("modified ruleset should be custom: defaults=%v custom=%v", gotDefaults, gotCustom)
	}
}
