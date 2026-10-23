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

package droppederrors

import (
	"fmt"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"golang.org/x/tools/go/analysis"
)

// Config holds runtime configuration for the droppederrors analyzer.
type Config struct {
	Policy             fileset.Policy
	Exclude            []string
	UseDefaultExcludes *bool
	isDefault          bool
}

var (
	skipGenerated bool
	skipTests     bool = true
)

// NewAnalyzer creates a new droppederrors Analyzer with the specified configuration.
func NewAnalyzer(cfg Config) *analysis.Analyzer {
	useDefaultExcludes := true
	if cfg.UseDefaultExcludes != nil {
		useDefaultExcludes = *cfg.UseDefaultExcludes
	}
	matcher := NewExclusionMatcher(cfg.Exclude, useDefaultExcludes)
	policy := cfg.Policy

	return &analysis.Analyzer{
		Name: "droppederrors",
		Doc:  "check for dropped errors, including errors assigned to blank identifier",
		Run: func(pass *analysis.Pass) (any, error) {
			effectivePolicy := policy
			if cfg.isDefault {
				effectivePolicy.SkipGenerated = skipGenerated
				effectivePolicy.SkipTests = skipTests
			}

			for _, file := range pass.Files {
				if effectivePolicy.ShouldSkipAST(pass.Fset, file) {
					continue
				}

				rawFindings := findDroppedErrorsInFile(file, pass.TypesInfo, matcher)
				for _, raw := range rawFindings {
					callee := formatCallee(raw.expr, pass.TypesInfo)
					enclosingFunc := findEnclosingFunc(file, pass.Fset, raw.pos)
					msg := fmt.Sprintf("unchecked error: %s", callee)
					if raw.category == CategoryErrorOnlyCheckedForSuccess {
						msg = fmt.Sprintf("error only checked for success: %s", callee)
					}
					pass.Report(analysis.Diagnostic{
						Pos:      raw.pos,
						Message:  msg,
						Category: fmt.Sprintf("%s:%s", raw.category, enclosingFunc),
						URL:      enclosingFunc,
					})
				}
			}
			return nil, nil
		},
	}
}

// Analyzer is the default droppederrors analyzer instance.
var Analyzer = NewAnalyzer(Config{Policy: fileset.NewPolicy(false, true, nil, ""), isDefault: true})

func init() {
	Analyzer.Flags.BoolVar(&skipGenerated, "skip-generated", false, "skip generated files")
	Analyzer.Flags.BoolVar(&skipTests, "skip-tests", true, "skip _test.go files")
}

// New constructs the droppederrors Analyzer from the resolved policy and loaded config.
func New(policy fileset.Policy, cfg *config.Config) *analysis.Analyzer {
	c := Config{Policy: policy}
	if cfg != nil {
		if de := cfg.DroppedErrors(); de != nil {
			c.Exclude = de.Exclude
			c.UseDefaultExcludes = de.UseDefaultExcludes
		}
	}
	return NewAnalyzer(c)
}
