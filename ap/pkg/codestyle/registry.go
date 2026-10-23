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

package codestyle

import (
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/any"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/closewriter"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/deferinloop"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/droppederrors"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/testcontext"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/testonlyimports"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/unused"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"golang.org/x/tools/go/analysis"
)

// Mode represents the reporting mode of an analyzer.
type Mode string

const (
	ModeError  Mode = "error"
	ModeWarn   Mode = "warn"
	ModeIgnore Mode = "ignore"
)

// Registered represents a registered analyzer and its defaults.
type Registered struct {
	Analyzer             *analysis.Analyzer
	Name                 string
	ConfigName           string
	Aliases              []string
	DefaultMode          Mode
	DefaultSkipGenerated bool
	DefaultSkipTests     bool
	CustomTask           bool
}

// ConfigKey returns the configuration key used in Config.Linter.
func (r *Registered) ConfigKey() string {
	if r.ConfigName != "" {
		return r.ConfigName
	}
	return r.Name
}

// Matches reports whether the registered analyzer matches the given name or any alias.
func (r *Registered) Matches(name string) bool {
	if strings.EqualFold(r.Name, name) {
		return true
	}
	if r.ConfigName != "" && strings.EqualFold(r.ConfigName, name) {
		return true
	}
	for _, a := range r.Aliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// ResolveMode returns the effective mode for this analyzer from config, falling back to DefaultMode.
func (r *Registered) ResolveMode(cfg *config.Config) Mode {
	if cfg == nil {
		return r.DefaultMode
	}
	lc := cfg.Linter(r.ConfigKey())
	if m := lc.GetMode(); m != "" {
		return Mode(m)
	}
	return r.DefaultMode
}

// ResolveSkipGenerated returns whether generated files should be skipped for this analyzer.
func (r *Registered) ResolveSkipGenerated(cfg *config.Config) bool {
	if cfg == nil {
		return r.DefaultSkipGenerated
	}
	lc := cfg.Linter(r.ConfigKey())
	return cfg.ResolveSkipGenerated(lc, r.DefaultSkipGenerated)
}

// ResolveSkipTests returns whether test files should be skipped for this analyzer.
func (r *Registered) ResolveSkipTests(cfg *config.Config) bool {
	if cfg == nil {
		return r.DefaultSkipTests
	}
	lc := cfg.Linter(r.ConfigKey())
	return cfg.ResolveSkipTests(lc, r.DefaultSkipTests)
}

// Analyzers is the central registry of code style analyzers.
var Analyzers = []Registered{
	{
		Analyzer:             unused.Analyzer,
		Name:                 "unused",
		ConfigName:           "unused",
		DefaultMode:          ModeError,
		DefaultSkipGenerated: true,
		DefaultSkipTests:     false,
	},
	{
		Analyzer:             testcontext.Analyzer,
		Name:                 "testcontext",
		ConfigName:           "testcontext",
		DefaultMode:          ModeWarn,
		DefaultSkipGenerated: false,
		DefaultSkipTests:     false,
	},
	{
		Analyzer:             any.Analyzer,
		Name:                 "replaceEmptyInterfaceWithAny",
		ConfigName:           "replaceEmptyInterfaceWithAny",
		Aliases:              []string{"replace-empty-interface-with-any"},
		DefaultMode:          ModeWarn,
		DefaultSkipGenerated: true,
		DefaultSkipTests:     false,
	},
	{
		Analyzer:             testonlyimports.Analyzer,
		Name:                 "testonlyimports",
		ConfigName:           "testOnlyImports",
		Aliases:              []string{"testOnlyImports"},
		DefaultMode:          ModeError,
		DefaultSkipGenerated: false,
		DefaultSkipTests:     false,
	},
	{
		Analyzer:             droppederrors.Analyzer,
		Name:                 "droppederrors",
		ConfigName:           "droppedErrors",
		Aliases:              []string{"droppedErrors"},
		DefaultMode:          ModeError,
		DefaultSkipGenerated: false,
		DefaultSkipTests:     true,
		CustomTask:           true,
	},
	{
		Analyzer:             closewriter.Analyzer,
		Name:                 "closewriter",
		ConfigName:           "closeWriter",
		Aliases:              []string{"closeWriter"},
		DefaultMode:          ModeError,
		DefaultSkipGenerated: false,
		DefaultSkipTests:     false,
	},
	{
		Analyzer:             deferinloop.Analyzer,
		Name:                 "deferinloop",
		ConfigName:           "deferInLoop",
		Aliases:              []string{"deferInLoop"},
		DefaultMode:          ModeError,
		DefaultSkipGenerated: false,
		DefaultSkipTests:     false,
	},
}

// Find finds a registered analyzer by name or alias.
func Find(name string) *Registered {
	for i := range Analyzers {
		if Analyzers[i].Matches(name) {
			return &Analyzers[i]
		}
	}
	return nil
}

// PlainAnalyzers returns the analyzers that run via standard multichecker/checker.
func PlainAnalyzers() []*analysis.Analyzer {
	var list []*analysis.Analyzer
	for _, r := range Analyzers {
		if !r.CustomTask && r.Analyzer != nil {
			list = append(list, r.Analyzer)
		}
	}
	return list
}
