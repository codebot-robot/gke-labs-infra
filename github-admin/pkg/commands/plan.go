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
	"fmt"
	"slices"
	"strings"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
	"github.com/google/go-github/v81/github"
)

// planSummary counts the changes found (and, when not in dry-run mode,
// made) across all repositories.
type planSummary struct {
	Add    int
	Change int
}

// fieldChange is a single scalar setting that differs from the config.
type fieldChange struct {
	Name string
	From string
	To   string
}

func (c fieldChange) String() string {
	return fmt.Sprintf("~ %s: %s => %s", c.Name, c.From, c.To)
}

func formatString(s *string) string {
	if s == nil {
		return "(unset)"
	}
	return fmt.Sprintf("%q", *s)
}

func formatBool(b *bool) string {
	if b == nil {
		return "(unset)"
	}
	return fmt.Sprintf("%t", *b)
}

// settingsChanges compares the scalar repository settings in cfg against the
// live repository. Only fields set in cfg are considered.
func settingsChanges(repo *github.Repository, cfg config.RepositoryConfig) []fieldChange {
	var out []fieldChange
	str := func(name string, cur, want *string) {
		if want != nil && (cur == nil || *cur != *want) {
			out = append(out, fieldChange{Name: name, From: formatString(cur), To: formatString(want)})
		}
	}
	boolean := func(name string, cur, want *bool) {
		if want != nil && (cur == nil || *cur != *want) {
			out = append(out, fieldChange{Name: name, From: formatBool(cur), To: formatBool(want)})
		}
	}

	str("description", repo.Description, cfg.Description)
	str("homepage", repo.Homepage, cfg.Homepage)
	boolean("private", repo.Private, cfg.Private)

	// Omitted settings mean their defaults, which are enforced.
	s := cfg.Settings.WithDefaults()
	{
		boolean("settings.allowAutoMerge", repo.AllowAutoMerge, s.AllowAutoMerge)
		boolean("settings.allowSquashMerge", repo.AllowSquashMerge, s.AllowSquashMerge)
		boolean("settings.allowMergeCommit", repo.AllowMergeCommit, s.AllowMergeCommit)
		boolean("settings.allowRebaseMerge", repo.AllowRebaseMerge, s.AllowRebaseMerge)
		boolean("settings.deleteBranchOnMerge", repo.DeleteBranchOnMerge, s.DeleteBranchOnMerge)
		str("settings.mergeCommitTitle", repo.MergeCommitTitle, s.MergeCommitTitle)
		str("settings.mergeCommitMessage", repo.MergeCommitMessage, s.MergeCommitMessage)
		boolean("settings.hasIssues", repo.HasIssues, s.HasIssues)
		boolean("settings.hasProjects", repo.HasProjects, s.HasProjects)
		boolean("settings.hasWiki", repo.HasWiki, s.HasWiki)
		boolean("settings.hasDownloads", repo.HasDownloads, s.HasDownloads)
	}
	return out
}

// topicsChange reports whether cfg.Topics differs from the live topics.
// An empty cfg.Topics means "unmanaged" and never produces a change.
func topicsChange(repo *github.Repository, cfg config.RepositoryConfig) (fieldChange, bool) {
	if len(cfg.Topics) == 0 {
		return fieldChange{}, false
	}
	cur := slices.Clone(repo.Topics)
	want := slices.Clone(cfg.Topics)
	slices.Sort(cur)
	slices.Sort(want)
	if slices.Equal(cur, want) {
		return fieldChange{}, false
	}
	return fieldChange{Name: "topics", From: fmt.Sprintf("%v", cur), To: fmt.Sprintf("%v", want)}, true
}

// desiredRulesetForDiff returns the desired ruleset as it will exist after
// applying it over current: fields that mean "leave unchanged" when omitted
// are filled in from current so that the diff only shows real changes.
func desiredRulesetForDiff(desired, current *config.RepositoryRuleset) *config.RepositoryRuleset {
	out := *desired
	if out.Target == "" {
		out.Target = "branch"
	}
	if out.BypassActors == nil && current != nil {
		out.BypassActors = current.BypassActors
	}
	return &out
}

// yamlLines renders v as YAML lines. A nil v yields no lines.
func yamlLines(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	data, err := MarshalYAML(v)
	if err != nil {
		return nil, err
	}
	s := strings.TrimRight(string(data), "\n")
	if s == "" || s == "{}" || s == "null" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

// diffLines produces a line diff of a against b, in the style of
// `diff`: unchanged lines are prefixed with two spaces, removed lines
// with "- " and added lines with "+ ". changed is false when a and b
// are identical.
func diffLines(a, b []string) (lines []string, changed bool) {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			lines = append(lines, "  "+a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			lines = append(lines, "- "+a[i])
			i++
			changed = true
		default:
			lines = append(lines, "+ "+b[j])
			j++
			changed = true
		}
	}
	for ; i < n; i++ {
		lines = append(lines, "- "+a[i])
		changed = true
	}
	for ; j < m; j++ {
		lines = append(lines, "+ "+b[j])
		changed = true
	}
	return lines, changed
}

// diffYAML renders current and desired as YAML and diffs them.
func diffYAML(current, desired any) (lines []string, changed bool, err error) {
	a, err := yamlLines(current)
	if err != nil {
		return nil, false, err
	}
	b, err := yamlLines(desired)
	if err != nil {
		return nil, false, err
	}
	lines, changed = diffLines(a, b)
	return lines, changed, nil
}

func printIndented(p *printer, indent string, lines []string) {
	if p == nil {
		return
	}
	for _, l := range lines {
		p.Printf("%s%s\n", indent, l)
	}
}
