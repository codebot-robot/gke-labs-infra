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
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
	"github.com/google/go-github/v81/github"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"sigs.k8s.io/yaml"
)

type ApplyOptions struct {
	ConfigPath         string
	DefaultRulesetsDir string
	GitHubToken        string
	DryRun             bool
	IncludePrivate     bool
}

func (o *ApplyOptions) InitDefaults() {
	o.DryRun = true
	o.DefaultRulesetsDir = DefaultRulesetsDir
}

func BuildApplyCommand() *cobra.Command {
	var opt ApplyOptions
	opt.InitDefaults()

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply github repo configurations from a file",
		Long: `Apply github repo configurations from a file or directory.

For each repository the current state is fetched and compared with the
configuration; only differences are printed, as a diff, and only those are
changed. With --dry-run (the default) nothing is modified.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("command does not take positional arguments")
			}
			return RunApply(cmd.Context(), opt)
		},
	}
	cmd.Flags().StringVar(&opt.ConfigPath, "config", opt.ConfigPath, "Path to a config file, or a directory of config files")
	cmd.Flags().StringVar(&opt.DefaultRulesetsDir, "default-rulesets", opt.DefaultRulesetsDir, "Directory holding the shared default rulesets")
	cmd.Flags().StringVar(&opt.GitHubToken, "token", opt.GitHubToken, "The github token (default from GITHUB_TOKEN env var)")
	cmd.Flags().BoolVar(&opt.DryRun, "dry-run", opt.DryRun, "If true, show the planned changes without making them")
	cmd.Flags().BoolVar(&opt.IncludePrivate, "include-private", opt.IncludePrivate, "Also apply to private repositories (skipped by default, as their configuration should not be committed)")

	return cmd
}

func RunApply(ctx context.Context, opt ApplyOptions) error {
	if opt.ConfigPath == "" {
		return fmt.Errorf("--config is required")
	}
	if opt.GitHubToken == "" {
		opt.GitHubToken = os.Getenv("GITHUB_TOKEN")
	}
	if opt.GitHubToken == "" {
		return fmt.Errorf("--token or GITHUB_TOKEN env var is required")
	}

	configs, err := LoadConfigs(opt.ConfigPath)
	if err != nil {
		return err
	}

	defaults, err := LoadDefaultRulesets(opt.DefaultRulesetsDir)
	if err != nil {
		return err
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opt.GitHubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	var summary planSummary
	var errs []error
	for _, cfg := range configs {
		if err := applyRepo(ctx, client, cfg, defaults, opt, &summary); err != nil {
			errs = append(errs, fmt.Errorf("error applying config to %s/%s: %w", cfg.Owner, cfg.Name, err))
		}
	}

	fmt.Println()
	switch {
	case summary.Add == 0 && summary.Change == 0:
		fmt.Println("No changes. GitHub matches the configuration.")
	case opt.DryRun:
		fmt.Printf("Plan: %d to add, %d to change. Dry run; re-run with --dry-run=false to apply.\n", summary.Add, summary.Change)
	default:
		fmt.Printf("Applied: %d added, %d changed.\n", summary.Add, summary.Change)
	}

	return errors.Join(errs...)
}

// LoadConfigs loads repository configs from path. If path is a directory,
// every .yaml/.yml file beneath it is loaded (in lexical order); otherwise
// path is read as a single (possibly multi-document) YAML file.
func LoadConfigs(path string) ([]config.RepositoryConfig, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config path: %w", err)
	}
	if !info.IsDir() {
		return loadConfigFile(path)
	}

	var configs []config.RepositoryConfig
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		fileConfigs, err := loadConfigFile(p)
		if err != nil {
			return err
		}
		configs = append(configs, fileConfigs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return configs, nil
}

func loadConfigFile(path string) ([]config.RepositoryConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configs []config.RepositoryConfig
	docs := SplitYAML(data)
	for _, doc := range docs {
		// Try unmarshal as single object
		var singleConfig config.RepositoryConfig
		if err := yaml.Unmarshal(doc, &singleConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config %s: %w", path, err)
		}
		configs = append(configs, singleConfig)
	}
	return configs, nil
}

func applyRepo(ctx context.Context, client *github.Client, cfg config.RepositoryConfig, defaults map[string]*config.RepositoryRuleset, opt ApplyOptions, summary *planSummary) error {
	out := os.Stdout
	fmt.Fprintf(out, "%s/%s\n", cfg.Owner, cfg.Name)

	repo, _, err := client.Repositories.Get(ctx, cfg.Owner, cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to get repo: %w", err)
	}
	if repo.GetPrivate() && !opt.IncludePrivate {
		fmt.Fprintf(out, "  skipped: repository is private (pass --include-private to manage it)\n")
		return nil
	}

	rulesets, err := resolveRulesets(&cfg, defaults)
	if err != nil {
		return err
	}

	changes := 0

	// Repository settings.
	fieldChanges := settingsChanges(repo, cfg)
	for _, c := range fieldChanges {
		fmt.Fprintf(out, "  %s\n", c)
	}
	if len(fieldChanges) > 0 {
		changes++
		if !opt.DryRun {
			repoReq := &github.Repository{
				Description: cfg.Description,
				Homepage:    cfg.Homepage,
				Private:     cfg.Private,
			}
			{
				s := cfg.Settings.WithDefaults()
				repoReq.AllowAutoMerge = s.AllowAutoMerge
				repoReq.AllowSquashMerge = s.AllowSquashMerge
				repoReq.AllowMergeCommit = s.AllowMergeCommit
				repoReq.AllowRebaseMerge = s.AllowRebaseMerge
				repoReq.DeleteBranchOnMerge = s.DeleteBranchOnMerge
				repoReq.MergeCommitTitle = s.MergeCommitTitle
				repoReq.MergeCommitMessage = s.MergeCommitMessage
				repoReq.HasIssues = s.HasIssues
				repoReq.HasProjects = s.HasProjects
				repoReq.HasWiki = s.HasWiki
				repoReq.HasDownloads = s.HasDownloads
			}
			if _, _, err := client.Repositories.Edit(ctx, cfg.Owner, cfg.Name, repoReq); err != nil {
				return fmt.Errorf("failed to edit repo: %w", err)
			}
			fmt.Fprintf(out, "    updated settings\n")
		}
	}

	topics, topicsChanged := topicsChange(repo, cfg)
	if len(fieldChanges) > 0 || topicsChanged {
		// The repository's own settings count as one changed object.
		summary.Change++
	}
	if topicsChanged {
		changes++
		fmt.Fprintf(out, "  %s\n", topics)
		if !opt.DryRun {
			if _, _, err := client.Repositories.ReplaceAllTopics(ctx, cfg.Owner, cfg.Name, cfg.Topics); err != nil {
				return fmt.Errorf("failed to update topics: %w", err)
			}
			fmt.Fprintf(out, "    updated topics\n")
		}
	}

	// Legacy branch protection.
	for branch, bp := range cfg.BranchProtection {
		var current *config.BranchProtection
		live, _, err := client.Repositories.GetBranchProtection(ctx, cfg.Owner, cfg.Name, branch)
		switch {
		case err == nil:
			current = mapBranchProtection(live)
		case errors.Is(err, github.ErrBranchNotProtected):
			// No current protection; everything is new.
		default:
			return fmt.Errorf("failed to get branch protection for %s: %w", branch, err)
		}

		lines, changed, err := diffYAML(current, bp)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		changes++
		if current == nil {
			summary.Add++
			fmt.Fprintf(out, "  + branchProtection %q\n", branch)
		} else {
			summary.Change++
			fmt.Fprintf(out, "  ~ branchProtection %q\n", branch)
		}
		printIndented(out, "      ", lines)

		if !opt.DryRun {
			if _, _, err := client.Repositories.UpdateBranchProtection(ctx, cfg.Owner, cfg.Name, branch, branchProtectionRequest(bp)); err != nil {
				return fmt.Errorf("failed to update branch protection for %s: %w", branch, err)
			}
			fmt.Fprintf(out, "    updated\n")
		}
	}

	// Rulesets.
	n, err := applyRulesets(ctx, client, out, cfg, rulesets, opt.DryRun, summary)
	if err != nil {
		return fmt.Errorf("failed to apply rulesets: %w", err)
	}
	changes += n

	if changes == 0 {
		fmt.Fprintf(out, "  no changes\n")
	}
	return nil
}

func branchProtectionRequest(bp *config.BranchProtection) *github.ProtectionRequest {
	req := &github.ProtectionRequest{
		EnforceAdmins:        bp.EnforceAdmins,
		RequireLinearHistory: &bp.RequireLinearHistory,
		AllowForcePushes:     &bp.AllowForcePushes,
		AllowDeletions:       &bp.AllowDeletions,
	}
	if bp.RequiredStatusChecks != nil {
		req.RequiredStatusChecks = &github.RequiredStatusChecks{
			Strict:   bp.RequiredStatusChecks.Strict,
			Contexts: &bp.RequiredStatusChecks.Contexts,
		}
	}
	if bp.RequiredPullRequestReviews != nil {
		req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{
			DismissStaleReviews:          bp.RequiredPullRequestReviews.DismissStaleReviews,
			RequireCodeOwnerReviews:      bp.RequiredPullRequestReviews.RequireCodeOwnerReviews,
			RequiredApprovingReviewCount: bp.RequiredPullRequestReviews.RequiredApprovingReviewCount,
		}
	}
	return req
}

// applyRulesets diffs each desired ruleset against GitHub, prints the
// differences, and (unless dryRun) creates or updates the ones that differ.
// It returns the number of rulesets that differed.
func applyRulesets(ctx context.Context, client *github.Client, out io.Writer, cfg config.RepositoryConfig, rulesets []*config.RepositoryRuleset, dryRun bool, summary *planSummary) (int, error) {
	existingRulesets, _, err := client.Repositories.GetAllRulesets(ctx, cfg.Owner, cfg.Name, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to list existing rulesets: %w", err)
	}

	existingMap := make(map[string]*github.RepositoryRuleset)
	for _, rs := range existingRulesets {
		// Only repository rulesets can be managed here.
		if st := rs.GetSourceType(); st != nil && *st != github.RulesetSourceTypeRepository {
			continue
		}
		existingMap[rs.Name] = rs
	}

	changes := 0
	for _, desired := range rulesets {
		rsReq := rulesetFromConfig(desired)

		existing, exists := existingMap[desired.Name]
		if !exists {
			lines, _, err := diffYAML(nil, desiredRulesetForDiff(desired, nil))
			if err != nil {
				return changes, err
			}
			changes++
			summary.Add++
			fmt.Fprintf(out, "  + ruleset %q\n", desired.Name)
			printIndented(out, "      ", lines)
			if dryRun {
				continue
			}
			err = withRetry(ctx, func() error {
				_, _, err := client.Repositories.CreateRuleset(ctx, cfg.Owner, cfg.Name, *rsReq)
				return err
			})
			if err != nil {
				return changes, fmt.Errorf("failed to create ruleset %s: %w", desired.Name, err)
			}
			fmt.Fprintf(out, "    created\n")
			continue
		}

		if existing.ID == nil {
			return changes, fmt.Errorf("existing ruleset %s has no ID", desired.Name)
		}
		full, _, err := client.Repositories.GetRuleset(ctx, cfg.Owner, cfg.Name, *existing.ID, false)
		if err != nil {
			return changes, fmt.Errorf("failed to get ruleset %s: %w", desired.Name, err)
		}
		current := mapRuleset(full)
		lines, changed, err := diffYAML(current, desiredRulesetForDiff(desired, current))
		if err != nil {
			return changes, err
		}
		if !changed {
			continue
		}
		changes++
		summary.Change++
		fmt.Fprintf(out, "  ~ ruleset %q\n", desired.Name)
		printIndented(out, "      ", lines)
		if dryRun {
			continue
		}
		err = withRetry(ctx, func() error {
			_, _, err := client.Repositories.UpdateRuleset(ctx, cfg.Owner, cfg.Name, *existing.ID, *rsReq)
			return err
		})
		if err != nil {
			return changes, fmt.Errorf("failed to update ruleset %s: %w", desired.Name, err)
		}
		fmt.Fprintf(out, "    updated\n")
	}
	return changes, nil
}

// withRetry retries fn when GitHub returns a 5xx error. The rulesets
// endpoints in particular have been observed to fail intermittently with
// 500/502 for requests that succeed when simply retried.
func withRetry(ctx context.Context, fn func() error) error {
	const maxAttempts = 5
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		var errResp *github.ErrorResponse
		if !errors.As(err, &errResp) || errResp.Response == nil || errResp.Response.StatusCode < 500 {
			return err
		}
		if attempt == maxAttempts {
			break
		}
		fmt.Fprintf(os.Stderr, "github returned %d; retrying (%d/%d)...\n", errResp.Response.StatusCode, attempt, maxAttempts)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 5 * time.Second):
		}
	}
	return err
}

func rulesetFromConfig(rs *config.RepositoryRuleset) *github.RepositoryRuleset {
	enforcement := github.RulesetEnforcement(rs.Enforcement)

	res := &github.RepositoryRuleset{
		Name:        rs.Name,
		Enforcement: enforcement,
	}

	if rs.Target != "" {
		target := github.RulesetTarget(rs.Target)
		res.Target = &target
	}

	if rs.Conditions != nil && rs.Conditions.RefName != nil {
		// GitHub rejects null for include/exclude; send empty lists instead.
		include := rs.Conditions.RefName.Include
		if include == nil {
			include = []string{}
		}
		exclude := rs.Conditions.RefName.Exclude
		if exclude == nil {
			exclude = []string{}
		}
		res.Conditions = &github.RepositoryRulesetConditions{
			RefName: &github.RepositoryRulesetRefConditionParameters{
				Include: include,
				Exclude: exclude,
			},
		}
	}

	if rs.BypassActors != nil {
		res.BypassActors = []*github.BypassActor{}
		for _, ba := range rs.BypassActors {
			actorType := github.BypassActorType(ba.ActorType)
			bypassMode := github.BypassMode(ba.BypassMode)
			actor := &github.BypassActor{
				ActorType:  &actorType,
				BypassMode: &bypassMode,
			}
			if ba.ActorID != 0 {
				actor.ActorID = github.Ptr(ba.ActorID)
			}
			res.BypassActors = append(res.BypassActors, actor)
		}
	}

	if rs.Rules != nil {
		res.Rules = &github.RepositoryRulesetRules{}
		if rs.Rules.Deletion {
			res.Rules.Deletion = &github.EmptyRuleParameters{}
		}
		if rs.Rules.NonFastForward {
			res.Rules.NonFastForward = &github.EmptyRuleParameters{}
		}
		if rs.Rules.PullRequest != nil {
			pr := rs.Rules.PullRequest
			var mergeMethods []github.PullRequestMergeMethod
			for _, m := range pr.AllowedMergeMethods {
				mergeMethods = append(mergeMethods, github.PullRequestMergeMethod(m))
			}
			res.Rules.PullRequest = &github.PullRequestRuleParameters{
				AllowedMergeMethods:            mergeMethods,
				DismissStaleReviewsOnPush:      pr.DismissStaleReviewsOnPush,
				RequireCodeOwnerReview:         pr.RequireCodeOwnerReview,
				RequireLastPushApproval:        pr.RequireLastPushApproval,
				RequiredApprovingReviewCount:   pr.RequiredApprovingReviewCount,
				RequiredReviewThreadResolution: pr.RequiredReviewThreadResolution,
			}
		}
		if rs.Rules.RequiredStatusChecks != nil {
			sc := rs.Rules.RequiredStatusChecks
			checks := []*github.RuleStatusCheck{}
			for _, c := range sc.Contexts {
				checks = append(checks, &github.RuleStatusCheck{Context: c})
			}
			res.Rules.RequiredStatusChecks = &github.RequiredStatusChecksRuleParameters{
				RequiredStatusChecks:             checks,
				StrictRequiredStatusChecksPolicy: sc.Strict,
			}
		}
		if rs.Rules.MergeQueue != nil {
			mq := rs.Rules.MergeQueue
			res.Rules.MergeQueue = &github.MergeQueueRuleParameters{
				CheckResponseTimeoutMinutes:  mq.CheckResponseTimeoutMinutes,
				GroupingStrategy:             github.MergeGroupingStrategy(mq.GroupingStrategy),
				MaxEntriesToBuild:            mq.MaxEntriesToBuild,
				MaxEntriesToMerge:            mq.MaxEntriesToMerge,
				MergeMethod:                  github.MergeQueueMergeMethod(mq.MergeMethod),
				MinEntriesToMerge:            mq.MinEntriesToMerge,
				MinEntriesToMergeWaitMinutes: mq.MinEntriesToMergeWaitMinutes,
			}
		}
	}
	return res
}
