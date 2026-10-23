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

package deferinloop

import (
	"go/ast"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const message = "defer inside a loop runs when the function returns, not each iteration; move the body into a function"

// NewAnalyzer creates a deferinloop analyzer configured with policy.
func NewAnalyzer(policy fileset.Policy) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "deferinloop",
		Doc:      "check for defer statements inside loops",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			return run(pass, policy)
		},
	}
}

func run(pass *analysis.Pass, policy fileset.Policy) (any, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	skippedFiles := make(map[*ast.File]bool, len(pass.Files))
	for _, f := range pass.Files {
		if policy.ShouldSkipAST(pass.Fset, f) {
			skippedFiles[f] = true
		}
	}

	nodeFilter := []ast.Node{
		(*ast.DeferStmt)(nil),
	}

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}

		if len(stack) > 0 {
			if file, ok := stack[0].(*ast.File); ok && skippedFiles[file] {
				return true
			}
		}

		deferStmt, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}

		for i := len(stack) - 2; i >= 0; i-- {
			ancestor := stack[i]
			switch ancestor.(type) {
			case *ast.FuncLit, *ast.FuncDecl:
				return true
			case *ast.ForStmt, *ast.RangeStmt:
				pass.Report(analysis.Diagnostic{
					Pos:      deferStmt.Pos(),
					Message:  message,
					Category: "deferinloop",
				})
				return true
			}
		}

		return true
	})

	return nil, nil
}
