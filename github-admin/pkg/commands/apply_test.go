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

func TestRulesetFromConfig(t *testing.T) {
	targetBranch := github.RulesetTarget("branch")

	tests := []struct {
		name string
		cfg  *config.RepositoryRuleset
		want *github.RepositoryRuleset
	}{
		{
			name: "Basic Ruleset",
			cfg: &config.RepositoryRuleset{
				Name:        "default",
				Target:      "branch",
				Enforcement: "active",
			},
			want: &github.RepositoryRuleset{
				Name:        "default",
				Target:      &targetBranch,
				Enforcement: github.RulesetEnforcement("active"),
			},
		},
		{
			name: "Ruleset with Merge Queue",
			cfg: &config.RepositoryRuleset{
				Name:        "merge-queue",
				Enforcement: "active",
				Rules: &config.RulesetRules{
					MergeQueue: &config.MergeQueueRule{
						MergeMethod:       "SQUASH",
						MinEntriesToMerge: 1,
					},
				},
			},
			want: &github.RepositoryRuleset{
				Name:        "merge-queue",
				Enforcement: "active",
				Rules: &github.RepositoryRulesetRules{
					MergeQueue: &github.MergeQueueRuleParameters{
						MergeMethod:       github.MergeQueueMergeMethod("SQUASH"),
						MinEntriesToMerge: 1,
					},
				},
			},
		},
		{
			name: "Ruleset with pull request, status checks and bypass actors",
			cfg: &config.RepositoryRuleset{
				Name:        "require-pr-reviews",
				Enforcement: "active",
				BypassActors: []config.BypassActor{
					{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"},
				},
				Rules: &config.RulesetRules{
					Deletion:       true,
					NonFastForward: true,
					PullRequest: &config.PullRequestRule{
						RequiredApprovingReviewCount: 1,
						DismissStaleReviewsOnPush:    true,
						AllowedMergeMethods:          []string{"merge", "squash"},
					},
					RequiredStatusChecks: &config.RequiredStatusChecks{
						Strict:   true,
						Contexts: []string{"ap-test"},
					},
				},
			},
			want: &github.RepositoryRuleset{
				Name:        "require-pr-reviews",
				Enforcement: "active",
				BypassActors: []*github.BypassActor{
					{
						ActorID:    github.Ptr(int64(5)),
						ActorType:  github.Ptr(github.BypassActorType("RepositoryRole")),
						BypassMode: github.Ptr(github.BypassMode("always")),
					},
				},
				Rules: &github.RepositoryRulesetRules{
					Deletion:       &github.EmptyRuleParameters{},
					NonFastForward: &github.EmptyRuleParameters{},
					PullRequest: &github.PullRequestRuleParameters{
						AllowedMergeMethods:          []github.PullRequestMergeMethod{"merge", "squash"},
						DismissStaleReviewsOnPush:    true,
						RequiredApprovingReviewCount: 1,
					},
					RequiredStatusChecks: &github.RequiredStatusChecksRuleParameters{
						RequiredStatusChecks:             []*github.RuleStatusCheck{{Context: "ap-test"}},
						StrictRequiredStatusChecksPolicy: true,
					},
				},
			},
		},
		{
			name: "Empty bypass actors clears them",
			cfg: &config.RepositoryRuleset{
				Name:         "merge-queue",
				Enforcement:  "active",
				BypassActors: []config.BypassActor{},
			},
			want: &github.RepositoryRuleset{
				Name:         "merge-queue",
				Enforcement:  "active",
				BypassActors: []*github.BypassActor{},
			},
		},
		{
			name: "Omitted exclude list is sent as empty, not null",
			cfg: &config.RepositoryRuleset{
				Name:        "defaults-only",
				Enforcement: "active",
				Conditions: &config.RulesetConditions{
					RefName: &config.RefNameCondition{Include: []string{"~DEFAULT_BRANCH"}},
				},
			},
			want: &github.RepositoryRuleset{
				Name:        "defaults-only",
				Enforcement: "active",
				Conditions: &github.RepositoryRulesetConditions{
					RefName: &github.RepositoryRulesetRefConditionParameters{
						Include: []string{"~DEFAULT_BRANCH"},
						Exclude: []string{},
					},
				},
			},
		},
		{
			name: "Ruleset with Conditions",
			cfg: &config.RepositoryRuleset{
				Name:        "main-protection",
				Enforcement: "active",
				Conditions: &config.RulesetConditions{
					RefName: &config.RefNameCondition{
						Include: []string{"refs/heads/main"},
						Exclude: []string{"refs/heads/dev"},
					},
				},
			},
			want: &github.RepositoryRuleset{
				Name:        "main-protection",
				Enforcement: "active",
				Conditions: &github.RepositoryRulesetConditions{
					RefName: &github.RepositoryRulesetRefConditionParameters{
						Include: []string{"refs/heads/main"},
						Exclude: []string{"refs/heads/dev"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rulesetFromConfig(tt.cfg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rulesetFromConfig() = \n%v\n, want \n%v", got, tt.want)
			}
		})
	}
}
