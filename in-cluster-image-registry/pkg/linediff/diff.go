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

package linediff

import (
	"fmt"
	"strings"
)

// DefaultContext is the default number of context lines to display around changes.
const DefaultContext = 2

// Diff returns a unified-style line diff between strings a and b.
// If a and b are identical, it returns an empty string.
func Diff(a, b string) string {
	return DiffWithContext(a, b, DefaultContext)
}

// DiffWithContext returns a unified-style line diff between a and b with the specified
// number of context lines around differences.
func DiffWithContext(a, b string, context int) string {
	if a == b {
		return ""
	}

	aLines := splitLines(a)
	bLines := splitLines(b)

	n, m := len(aLines), len(bLines)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if aLines[i] == bLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	type op struct {
		kind  byte // ' ', '-', '+'
		text  string
		aIdx  int // 1-based line number in a (0 if not in a)
		bIdx  int // 1-based line number in b (0 if not in b)
		prevA int // line number in a before this op
		prevB int // line number in b before this op
	}

	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		prevA, prevB := i, j
		switch {
		case aLines[i] == bLines[j]:
			ops = append(ops, op{kind: ' ', text: aLines[i], aIdx: i + 1, bIdx: j + 1, prevA: prevA, prevB: prevB})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{kind: '-', text: aLines[i], aIdx: i + 1, prevA: prevA, prevB: prevB})
			i++
		default:
			ops = append(ops, op{kind: '+', text: bLines[j], bIdx: j + 1, prevA: prevA, prevB: prevB})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{kind: '-', text: aLines[i], aIdx: i + 1, prevA: i, prevB: j})
	}
	for ; j < m; j++ {
		ops = append(ops, op{kind: '+', text: bLines[j], bIdx: j + 1, prevA: i, prevB: j})
	}

	include := make([]bool, len(ops))
	hasChange := false
	for idx, o := range ops {
		if o.kind != ' ' {
			hasChange = true
			start := max(0, idx-context)
			end := min(len(ops), idx+context+1)
			for k := start; k < end; k++ {
				include[k] = true
			}
		}
	}

	if !hasChange {
		return ""
	}

	var hunks [][]op
	var currentHunk []op
	for idx, inc := range include {
		if inc {
			currentHunk = append(currentHunk, ops[idx])
		} else if len(currentHunk) > 0 {
			hunks = append(hunks, currentHunk)
			currentHunk = nil
		}
	}
	if len(currentHunk) > 0 {
		hunks = append(hunks, currentHunk)
	}

	var out []string
	for _, hunk := range hunks {
		var aLinesInHunk, bLinesInHunk []op
		for _, o := range hunk {
			if o.kind == ' ' || o.kind == '-' {
				aLinesInHunk = append(aLinesInHunk, o)
			}
			if o.kind == ' ' || o.kind == '+' {
				bLinesInHunk = append(bLinesInHunk, o)
			}
		}

		aStart := hunk[0].prevA
		if len(aLinesInHunk) > 0 {
			aStart = aLinesInHunk[0].aIdx
		}
		bStart := hunk[0].prevB
		if len(bLinesInHunk) > 0 {
			bStart = bLinesInHunk[0].bIdx
		}

		out = append(out, fmt.Sprintf("@@ -%s +%s @@", formatRange(aStart, len(aLinesInHunk)), formatRange(bStart, len(bLinesInHunk))))
		for _, o := range hunk {
			out = append(out, fmt.Sprintf("%c%s", o.kind, o.text))
		}
	}

	return strings.Join(out, "\n")
}

func formatRange(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\r\n")
	s = strings.TrimSuffix(s, "\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}
