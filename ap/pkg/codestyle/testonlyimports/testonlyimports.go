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

package testonlyimports

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"golang.org/x/tools/go/analysis"
)

// DefaultPackages is the default list of test-only package import prefixes.
var DefaultPackages = []string{
	"github.com/google/go-cmp",
	"github.com/stretchr/testify",
	"golang.org/x/tools/go/analysis/analysistest",
	"k8s.io/client-go/kubernetes/fake",
}

// Config holds configuration options for the testonlyimports analyzer.
type Config struct {
	fileset.Policy
	Packages []string
}

// NewAnalyzer creates a testonlyimports analyzer configured with cfg.
func NewAnalyzer(cfg Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "testonlyimports",
		Doc:  "check for imports of test-only packages from non-test Go code",
		Run: func(pass *analysis.Pass) (any, error) {
			return run(pass, cfg)
		},
	}
}

func run(pass *analysis.Pass, cfg Config) (any, error) {
	packages := DefaultPackages
	if len(cfg.Packages) > 0 {
		packages = cfg.Packages
	}

	for _, f := range pass.Files {
		if cfg.Policy.ShouldSkipAST(pass.Fset, f) {
			continue
		}

		filename := pass.Fset.Position(f.Pos()).Filename
		if isTestFile(filename) {
			continue
		}

		for _, imp := range f.Imports {
			if imp.Path == nil {
				continue
			}
			pathVal, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				pathVal = strings.Trim(imp.Path.Value, `"`)
			}
			if isTestOnly(pathVal, packages) {
				pass.Reportf(imp.Pos(), "import of test-only package %s from non-test file", pathVal)
			}
		}
	}
	return nil, nil
}

// isTestFile reports whether filename is a test file (_test.go) or is under a testdata/ directory.
// Files under analysistest's harness root (testdata/src/<pkg>/...) are not exempt unless they are _test.go
// or under a nested testdata/ directory.
func isTestFile(filename string) bool {
	if fileset.IsTest(filename) {
		return true
	}
	slash := filepath.ToSlash(filepath.Clean(filename))
	parts := strings.Split(slash, "/")
	for i, part := range parts {
		if part == "testdata" {
			if i+1 < len(parts) && parts[i+1] == "src" {
				continue
			}
			return true
		}
	}
	return false
}

// isTestOnly reports whether importPath equals or has the prefix of any test-only package.
func isTestOnly(importPath string, testOnlyPackages []string) bool {
	for _, pkg := range testOnlyPackages {
		pkg = strings.TrimSuffix(pkg, "/")
		if importPath == pkg || strings.HasPrefix(importPath, pkg+"/") {
			return true
		}
	}
	return false
}
