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

// RepositoryConfig represents the configuration of a GitHub repository.
type RepositoryConfig struct {
	// Owner is the GitHub organization or user.
	// +optional
	Owner string `json:"owner,omitempty" yaml:"owner,omitempty"`

	// Name is the name of the repository.
	Name string `json:"name" yaml:"name"`

	// Description is the repository description.
	// +optional
	Description *string `json:"description,omitempty" yaml:"description,omitempty"`

	// Homepage is the repository homepage URL.
	// +optional
	Homepage *string `json:"homepage,omitempty" yaml:"homepage,omitempty"`

	// Private indicates if the repository is private.
	// +optional
	Private *bool `json:"private,omitempty" yaml:"private,omitempty"`

	// Topics is a list of topics.
	// +optional
	Topics []string `json:"topics,omitempty" yaml:"topics,omitempty"`

	// Settings contains repository settings.
	// +optional
	Settings *RepositorySettings `json:"settings,omitempty" yaml:"settings,omitempty"`

	// BranchProtection defines protection rules for branches.
	// The key is the branch pattern (e.g., "main").
	// +optional
	BranchProtection map[string]*BranchProtection `json:"branchProtection,omitempty" yaml:"branchProtection,omitempty"`

	// DefaultRulesets names shared rulesets, defined once in the default
	// rulesets directory, that are applied to this repository verbatim.
	// +optional
	DefaultRulesets []string `json:"defaultRulesets,omitempty" yaml:"defaultRulesets,omitempty"`

	// CustomRulesets defines rulesets specific to this repository.
	// +optional
	CustomRulesets []*RepositoryRuleset `json:"customRulesets,omitempty" yaml:"customRulesets,omitempty"`
}

type RepositorySettings struct {
	AllowAutoMerge      *bool `json:"allowAutoMerge,omitempty" yaml:"allowAutoMerge,omitempty"`
	AllowSquashMerge    *bool `json:"allowSquashMerge,omitempty" yaml:"allowSquashMerge,omitempty"`
	AllowMergeCommit    *bool `json:"allowMergeCommit,omitempty" yaml:"allowMergeCommit,omitempty"`
	AllowRebaseMerge    *bool `json:"allowRebaseMerge,omitempty" yaml:"allowRebaseMerge,omitempty"`
	DeleteBranchOnMerge *bool `json:"deleteBranchOnMerge,omitempty" yaml:"deleteBranchOnMerge,omitempty"`

	MergeCommitTitle   *string `json:"mergeCommitTitle,omitempty" yaml:"mergeCommitTitle,omitempty"`
	MergeCommitMessage *string `json:"mergeCommitMessage,omitempty" yaml:"mergeCommitMessage,omitempty"`

	HasIssues    *bool `json:"hasIssues,omitempty" yaml:"hasIssues,omitempty"`
	HasProjects  *bool `json:"hasProjects,omitempty" yaml:"hasProjects,omitempty"`
	HasWiki      *bool `json:"hasWiki,omitempty" yaml:"hasWiki,omitempty"`
	HasDownloads *bool `json:"hasDownloads,omitempty" yaml:"hasDownloads,omitempty"`
}

type BranchProtection struct {
	RequiredStatusChecks       *RequiredStatusChecks       `json:"requiredStatusChecks,omitempty" yaml:"requiredStatusChecks,omitempty"`
	RequiredPullRequestReviews *RequiredPullRequestReviews `json:"requiredPullRequestReviews,omitempty" yaml:"requiredPullRequestReviews,omitempty"`
	EnforceAdmins              bool                        `json:"enforceAdmins,omitempty" yaml:"enforceAdmins,omitempty"`
	RequireLinearHistory       bool                        `json:"requireLinearHistory,omitempty" yaml:"requireLinearHistory,omitempty"`
	AllowForcePushes           bool                        `json:"allowForcePushes,omitempty" yaml:"allowForcePushes,omitempty"`
	AllowDeletions             bool                        `json:"allowDeletions,omitempty" yaml:"allowDeletions,omitempty"`
}

type RepositoryRuleset struct {
	Name        string             `json:"name,omitempty" yaml:"name,omitempty"`
	Target      string             `json:"target,omitempty" yaml:"target,omitempty"`
	Enforcement string             `json:"enforcement,omitempty" yaml:"enforcement,omitempty"`
	Conditions  *RulesetConditions `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Rules       *RulesetRules      `json:"rules,omitempty" yaml:"rules,omitempty"`

	// BypassActors lists who may bypass this ruleset.
	// When nil, existing bypass actors on the ruleset are left unchanged;
	// use an empty list to clear them.
	// +optional
	BypassActors []BypassActor `json:"bypassActors,omitempty" yaml:"bypassActors,omitempty"`
}

// BypassActor identifies an actor that can bypass a ruleset.
type BypassActor struct {
	// ActorID is the ID of the actor. For RepositoryRole it is the role ID
	// (1 = read, 2 = triage, 4 = write, 5 = admin); for Team/Integration/User it
	// is the respective object ID. It is unused for OrganizationAdmin.
	ActorID int64 `json:"actorId,omitempty" yaml:"actorId,omitempty"`
	// ActorType is one of RepositoryRole, Team, Integration, OrganizationAdmin, User.
	ActorType string `json:"actorType,omitempty" yaml:"actorType,omitempty"`
	// BypassMode is one of always, pull_request.
	BypassMode string `json:"bypassMode,omitempty" yaml:"bypassMode,omitempty"`
}

type RulesetConditions struct {
	RefName *RefNameCondition `json:"refName,omitempty" yaml:"refName,omitempty"`
}

type RefNameCondition struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

type RulesetRules struct {
	// Deletion prevents the matching refs from being deleted.
	Deletion bool `json:"deletion,omitempty" yaml:"deletion,omitempty"`
	// NonFastForward prevents force pushes to the matching refs.
	NonFastForward bool `json:"nonFastForward,omitempty" yaml:"nonFastForward,omitempty"`

	MergeQueue           *MergeQueueRule       `json:"mergeQueue,omitempty" yaml:"mergeQueue,omitempty"`
	PullRequest          *PullRequestRule      `json:"pullRequest,omitempty" yaml:"pullRequest,omitempty"`
	RequiredStatusChecks *RequiredStatusChecks `json:"requiredStatusChecks,omitempty" yaml:"requiredStatusChecks,omitempty"`
}

// PullRequestRule requires changes to be made via a pull request.
type PullRequestRule struct {
	RequiredApprovingReviewCount   int      `json:"requiredApprovingReviewCount,omitempty" yaml:"requiredApprovingReviewCount,omitempty"`
	DismissStaleReviewsOnPush      bool     `json:"dismissStaleReviewsOnPush,omitempty" yaml:"dismissStaleReviewsOnPush,omitempty"`
	RequireCodeOwnerReview         bool     `json:"requireCodeOwnerReview,omitempty" yaml:"requireCodeOwnerReview,omitempty"`
	RequireLastPushApproval        bool     `json:"requireLastPushApproval,omitempty" yaml:"requireLastPushApproval,omitempty"`
	RequiredReviewThreadResolution bool     `json:"requiredReviewThreadResolution,omitempty" yaml:"requiredReviewThreadResolution,omitempty"`
	AllowedMergeMethods            []string `json:"allowedMergeMethods,omitempty" yaml:"allowedMergeMethods,omitempty"`
}

type MergeQueueRule struct {
	CheckResponseTimeoutMinutes  int    `json:"checkResponseTimeoutMinutes,omitempty" yaml:"checkResponseTimeoutMinutes,omitempty"`
	GroupingStrategy             string `json:"groupingStrategy,omitempty" yaml:"groupingStrategy,omitempty"`
	MaxEntriesToBuild            int    `json:"maxEntriesToBuild,omitempty" yaml:"maxEntriesToBuild,omitempty"`
	MaxEntriesToMerge            int    `json:"maxEntriesToMerge,omitempty" yaml:"maxEntriesToMerge,omitempty"`
	MergeMethod                  string `json:"mergeMethod,omitempty" yaml:"mergeMethod,omitempty"`
	MinEntriesToMerge            int    `json:"minEntriesToMerge,omitempty" yaml:"minEntriesToMerge,omitempty"`
	MinEntriesToMergeWaitMinutes int    `json:"minEntriesToMergeWaitMinutes,omitempty" yaml:"minEntriesToMergeWaitMinutes,omitempty"`
}

type RequiredStatusChecks struct {
	Strict   bool     `json:"strict,omitempty" yaml:"strict,omitempty"`
	Contexts []string `json:"contexts,omitempty" yaml:"contexts,omitempty"`
}

type RequiredPullRequestReviews struct {
	DismissStaleReviews          bool `json:"dismissStaleReviews,omitempty" yaml:"dismissStaleReviews,omitempty"`
	RequireCodeOwnerReviews      bool `json:"requireCodeOwnerReviews,omitempty" yaml:"requireCodeOwnerReviews,omitempty"`
	RequiredApprovingReviewCount int  `json:"requiredApprovingReviewCount,omitempty" yaml:"requiredApprovingReviewCount,omitempty"`
}

// DefaultRepositorySettings are GitHub's defaults for a new repository. A
// setting omitted from a RepositoryConfig means its default, and apply
// enforces it; export omits settings that equal their default.
func DefaultRepositorySettings() RepositorySettings {
	return RepositorySettings{
		AllowAutoMerge:      ptr(false),
		AllowSquashMerge:    ptr(true),
		AllowMergeCommit:    ptr(true),
		AllowRebaseMerge:    ptr(true),
		DeleteBranchOnMerge: ptr(false),
		MergeCommitTitle:    ptr("MERGE_MESSAGE"),
		MergeCommitMessage:  ptr("PR_TITLE"),
		HasIssues:           ptr(true),
		HasProjects:         ptr(true),
		HasWiki:             ptr(true),
		HasDownloads:        ptr(false),
	}
}

// WithDefaults returns a copy of s (which may be nil) with every unset field
// filled in from DefaultRepositorySettings.
func (s *RepositorySettings) WithDefaults() RepositorySettings {
	out := DefaultRepositorySettings()
	if s == nil {
		return out
	}
	override(&out.AllowAutoMerge, s.AllowAutoMerge)
	override(&out.AllowSquashMerge, s.AllowSquashMerge)
	override(&out.AllowMergeCommit, s.AllowMergeCommit)
	override(&out.AllowRebaseMerge, s.AllowRebaseMerge)
	override(&out.DeleteBranchOnMerge, s.DeleteBranchOnMerge)
	override(&out.MergeCommitTitle, s.MergeCommitTitle)
	override(&out.MergeCommitMessage, s.MergeCommitMessage)
	override(&out.HasIssues, s.HasIssues)
	override(&out.HasProjects, s.HasProjects)
	override(&out.HasWiki, s.HasWiki)
	override(&out.HasDownloads, s.HasDownloads)
	return out
}

// WithoutDefaults returns a copy of s with every field that equals its
// default unset, or nil if no field differs from the defaults.
func (s *RepositorySettings) WithoutDefaults() *RepositorySettings {
	if s == nil {
		return nil
	}
	def := DefaultRepositorySettings()
	out := &RepositorySettings{
		AllowAutoMerge:      nonDefault(s.AllowAutoMerge, def.AllowAutoMerge),
		AllowSquashMerge:    nonDefault(s.AllowSquashMerge, def.AllowSquashMerge),
		AllowMergeCommit:    nonDefault(s.AllowMergeCommit, def.AllowMergeCommit),
		AllowRebaseMerge:    nonDefault(s.AllowRebaseMerge, def.AllowRebaseMerge),
		DeleteBranchOnMerge: nonDefault(s.DeleteBranchOnMerge, def.DeleteBranchOnMerge),
		MergeCommitTitle:    nonDefault(s.MergeCommitTitle, def.MergeCommitTitle),
		MergeCommitMessage:  nonDefault(s.MergeCommitMessage, def.MergeCommitMessage),
		HasIssues:           nonDefault(s.HasIssues, def.HasIssues),
		HasProjects:         nonDefault(s.HasProjects, def.HasProjects),
		HasWiki:             nonDefault(s.HasWiki, def.HasWiki),
		HasDownloads:        nonDefault(s.HasDownloads, def.HasDownloads),
	}
	if *out == (RepositorySettings{}) {
		return nil
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func override[T any](dst **T, v *T) {
	if v != nil {
		*dst = v
	}
}

// nonDefault returns v unless it is unset or equal to def.
func nonDefault[T comparable](v, def *T) *T {
	if v == nil || (def != nil && *v == *def) {
		return nil
	}
	return v
}
