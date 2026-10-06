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
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gke-labs/gke-labs-infra/github-admin/pkg/config"
	"sigs.k8s.io/yaml"
)

// DefaultRulesetsDir is where the shared default rulesets live, relative to
// the repository root.
const DefaultRulesetsDir = "github-admin/rulesets"

// LoadDefaultRulesets loads the shared rulesets from dir, keyed by name.
// Each .yaml/.yml file holds one ruleset whose name must match the file
// name (without extension). A missing directory yields an empty map.
func LoadDefaultRulesets(dir string) (map[string]*config.RepositoryRuleset, error) {
	defaults := make(map[string]*config.RepositoryRuleset)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaults, nil
		}
		return nil, fmt.Errorf("failed to read default rulesets directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read default ruleset %s: %w", path, err)
		}
		rs := &config.RepositoryRuleset{}
		if err := yaml.UnmarshalStrict(data, rs); err != nil {
			return nil, fmt.Errorf("failed to parse default ruleset %s: %w", path, err)
		}
		want := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if rs.Name != want {
			return nil, fmt.Errorf("default ruleset %s has name %q; it must match the file name %q", path, rs.Name, want)
		}
		defaults[rs.Name] = rs
	}
	return defaults, nil
}

// resolveRulesets returns the full list of rulesets to apply to cfg: the
// referenced defaults (in order) followed by the custom rulesets. Names
// must be unique across both.
func resolveRulesets(cfg *config.RepositoryConfig, defaults map[string]*config.RepositoryRuleset) ([]*config.RepositoryRuleset, error) {
	var out []*config.RepositoryRuleset
	seen := make(map[string]bool)

	for _, name := range cfg.DefaultRulesets {
		rs, ok := defaults[name]
		if !ok {
			return nil, fmt.Errorf("repo %s references unknown default ruleset %q", cfg.Name, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("repo %s lists default ruleset %q more than once", cfg.Name, name)
		}
		seen[name] = true
		out = append(out, rs)
	}

	for _, rs := range cfg.CustomRulesets {
		if rs.Name == "" {
			return nil, fmt.Errorf("repo %s has a custom ruleset with no name", cfg.Name)
		}
		if seen[rs.Name] {
			return nil, fmt.Errorf("repo %s defines ruleset %q as both a default and a custom ruleset", cfg.Name, rs.Name)
		}
		seen[rs.Name] = true
		out = append(out, rs)
	}
	return out, nil
}

// classifyRulesets splits live rulesets into those that exactly match a
// default (returned by name) and the rest (returned in full).
func classifyRulesets(live []*config.RepositoryRuleset, defaults map[string]*config.RepositoryRuleset) ([]string, []*config.RepositoryRuleset) {
	var defaultNames []string
	var custom []*config.RepositoryRuleset
	for _, rs := range live {
		if def, ok := defaults[rs.Name]; ok && rulesetsEqual(rs, def) {
			defaultNames = append(defaultNames, rs.Name)
			continue
		}
		custom = append(custom, rs)
	}
	return defaultNames, custom
}

// rulesetsEqual reports whether two rulesets are semantically identical.
// Comparison is done on the serialized form so that empty and nil slices,
// and unset and zero-valued fields, compare equal.
func rulesetsEqual(a, b *config.RepositoryRuleset) bool {
	aj, err := yaml.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := yaml.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}
