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
	"reflect"
	"testing"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
	"github.com/google/go-github/v81/github"
)

func TestDiffLines(t *testing.T) {
	tests := []struct {
		name        string
		a, b        []string
		want        []string
		wantChanged bool
	}{
		{
			name: "identical",
			a:    []string{"x: 1", "y: 2"},
			b:    []string{"x: 1", "y: 2"},
			want: []string{"  x: 1", "  y: 2"},
		},
		{
			name:        "create",
			a:           nil,
			b:           []string{"x: 1"},
			want:        []string{"+ x: 1"},
			wantChanged: true,
		},
		{
			name:        "change in the middle",
			a:           []string{"a", "b", "c"},
			b:           []string{"a", "B", "c"},
			want:        []string{"  a", "- b", "+ B", "  c"},
			wantChanged: true,
		},
		{
			name:        "removal at end",
			a:           []string{"a", "b"},
			b:           []string{"a"},
			want:        []string{"  a", "- b"},
			wantChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := diffLines(tt.a, tt.b)
			if changed != tt.wantChanged || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("diffLines() = %q, %v; want %q, %v", got, changed, tt.want, tt.wantChanged)
			}
		})
	}
}

func TestSettingsChanges(t *testing.T) {
	defaults := config.DefaultRepositorySettings()
	atDefaults := &github.Repository{
		AllowAutoMerge:      defaults.AllowAutoMerge,
		AllowSquashMerge:    defaults.AllowSquashMerge,
		AllowMergeCommit:    defaults.AllowMergeCommit,
		AllowRebaseMerge:    defaults.AllowRebaseMerge,
		DeleteBranchOnMerge: defaults.DeleteBranchOnMerge,
		MergeCommitTitle:    defaults.MergeCommitTitle,
		MergeCommitMessage:  defaults.MergeCommitMessage,
		HasIssues:           defaults.HasIssues,
		HasProjects:         defaults.HasProjects,
		HasWiki:             defaults.HasWiki,
		HasDownloads:        defaults.HasDownloads,
	}

	t.Run("repo at defaults with empty config has no changes", func(t *testing.T) {
		if got := settingsChanges(atDefaults, config.RepositoryConfig{}); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("omitted setting is enforced to its default", func(t *testing.T) {
		repo := *atDefaults
		repo.HasWiki = github.Ptr(false)
		got := settingsChanges(&repo, config.RepositoryConfig{})
		want := []fieldChange{{Name: "settings.hasWiki", From: "false", To: "true"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("only differing fields are reported", func(t *testing.T) {
		repo := *atDefaults
		repo.Description = github.Ptr("old")
		cfg := config.RepositoryConfig{
			Description: github.Ptr("old"), // same
			Settings: &config.RepositorySettings{
				AllowAutoMerge: github.Ptr(true), // differs from repo
				HasWiki:        github.Ptr(true), // same
			},
		}
		got := settingsChanges(&repo, cfg)
		want := []fieldChange{{Name: "settings.allowAutoMerge", From: "false", To: "true"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("unmanaged description is ignored", func(t *testing.T) {
		repo := *atDefaults
		repo.Description = github.Ptr("something")
		if got := settingsChanges(&repo, config.RepositoryConfig{}); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}

func TestMarshalYAMLFieldOrder(t *testing.T) {
	cfg := config.RepositoryConfig{
		Owner:           "org",
		Name:            "repo",
		Settings:        &config.RepositorySettings{HasWiki: github.Ptr(false)},
		DefaultRulesets: []string{"require-pr-reviews"},
	}
	data, err := MarshalYAML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "owner: org\nname: repo\nsettings:\n  hasWiki: false\ndefaultRulesets:\n  - require-pr-reviews\n"
	if string(data) != want {
		t.Errorf("MarshalYAML() =\n%s\nwant\n%s", data, want)
	}
}

func TestTopicsChange(t *testing.T) {
	repo := &github.Repository{Topics: []string{"b", "a"}}
	if _, changed := topicsChange(repo, config.RepositoryConfig{}); changed {
		t.Error("empty topics should be unmanaged")
	}
	if _, changed := topicsChange(repo, config.RepositoryConfig{Topics: []string{"a", "b"}}); changed {
		t.Error("same set in different order should not change")
	}
	if c, changed := topicsChange(repo, config.RepositoryConfig{Topics: []string{"a"}}); !changed || c.Name != "topics" {
		t.Errorf("expected change, got %v %v", c, changed)
	}
}

func TestRulesetDiffIgnoresUnmanagedFields(t *testing.T) {
	// As exported from GitHub: empty exclude list, explicit target and
	// bypass actors.
	current := &config.RepositoryRuleset{
		Name: "require-pr-reviews", Target: "branch", Enforcement: "active",
		Conditions: &config.RulesetConditions{
			RefName: &config.RefNameCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}},
		},
		BypassActors: []config.BypassActor{{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"}},
		Rules:        &config.RulesetRules{Deletion: true},
	}
	// As a user would write it: no target, no exclude, bypass actors omitted.
	desired := &config.RepositoryRuleset{
		Name: "require-pr-reviews", Enforcement: "active",
		Conditions: &config.RulesetConditions{RefName: &config.RefNameCondition{Include: []string{"~DEFAULT_BRANCH"}}},
		Rules:      &config.RulesetRules{Deletion: true},
	}

	_, changed, err := diffYAML(current, desiredRulesetForDiff(desired, current))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected no change when desired only omits unmanaged fields")
	}

	desired.Rules.NonFastForward = true
	lines, changed, err := diffYAML(current, desiredRulesetForDiff(desired, current))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("expected change, got %q", lines)
	}
	found := false
	for _, l := range lines {
		if l == "+   nonFastForward: true" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected added nonFastForward line, got %q", lines)
	}
}
