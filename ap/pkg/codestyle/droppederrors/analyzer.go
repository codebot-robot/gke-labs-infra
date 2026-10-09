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
	"strings"

	"golang.org/x/tools/go/analysis"
)

var (
	analyzerSkipTests = true
)

// Analyzer checks for unchecked/dropped errors with blank checking enabled.
var Analyzer = &analysis.Analyzer{
	Name: "droppederrors",
	Doc:  "check for dropped errors, including errors assigned to blank identifier",
	Run:  runAnalyzer,
}

func init() {
	Analyzer.Flags.BoolVar(&analyzerSkipTests, "skip-tests", true, "skip _test.go files")
}

func runAnalyzer(pass *analysis.Pass) (any, error) {
	matcher := NewExclusionMatcher(nil, true)

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if analyzerSkipTests && strings.HasSuffix(filename, "_test.go") {
			continue
		}

		rawFindings := findDroppedErrorsInFile(file, pass.TypesInfo, matcher)
		for _, raw := range rawFindings {
			callee := formatCallee(raw.expr, pass.TypesInfo)
			msg := fmt.Sprintf("unchecked error: %s", callee)
			if raw.category == CategoryErrorOnlyCheckedForSuccess {
				msg = fmt.Sprintf("error only checked for success: %s", callee)
			}
			pass.Report(analysis.Diagnostic{
				Pos:      raw.pos,
				Message:  msg,
				Category: "droppederrors",
			})
		}
	}

	return nil, nil
}
