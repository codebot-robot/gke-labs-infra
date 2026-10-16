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

// Package linediff provides lightweight, dependency-free unified line diffing.
// It trims common prefix and suffix lines before running LCS to handle large
// inputs with localized changes efficiently. If the remaining changed region
// exceeds MaxTableCells, DiffWithContext falls back to a degraded summary
// instead of allocating an excessive LCS table.
package linediff

import (
	"fmt"
	"strings"
)

// DefaultContext is the default number of context lines to display around changes.
const DefaultContext = 2

// MaxTableCells is the maximum number of cells in the LCS table before DiffWithContext
// falls back to a degraded summary to avoid excessive memory allocation.
const MaxTableCells = 4_000_000

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

	prefix := 0
	for prefix < len(aLines) && prefix < len(bLines) && aLines[prefix] == bLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(aLines)-prefix && suffix < len(bLines)-prefix && aLines[len(aLines)-1-suffix] == bLines[len(bLines)-1-suffix] {
		suffix++
	}

	if prefix == len(aLines) && prefix == len(bLines) {
		return ""
	}

	midA := aLines[prefix : len(aLines)-suffix]
	midB := bLines[prefix : len(bLines)-suffix]
	n, m := len(midA), len(midB)

	if int64(n)*int64(m) > MaxTableCells {
		return degradedDiff(midA, midB, prefix, n, m)
	}

	var lcs [][]int
	if n > 0 && m > 0 {
		lcs = make([][]int, n+1)
		for i := range lcs {
			lcs[i] = make([]int, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if midA[i] == midB[j] {
					lcs[i][j] = lcs[i+1][j+1] + 1
				} else {
					lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
				}
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

	pContext := max(0, min(prefix, context))
	sContext := max(0, min(suffix, context))

	var ops []op
	for k := prefix - pContext; k < prefix; k++ {
		ops = append(ops, op{kind: ' ', text: aLines[k], aIdx: k + 1, bIdx: k + 1, prevA: k, prevB: k})
	}

	i, j := 0, 0
	for i < n && j < m {
		prevA, prevB := prefix+i, prefix+j
		switch {
		case midA[i] == midB[j]:
			ops = append(ops, op{kind: ' ', text: midA[i], aIdx: prevA + 1, bIdx: prevB + 1, prevA: prevA, prevB: prevB})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{kind: '-', text: midA[i], aIdx: prevA + 1, prevA: prevA, prevB: prevB})
			i++
		default:
			ops = append(ops, op{kind: '+', text: midB[j], bIdx: prevB + 1, prevA: prevA, prevB: prevB})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{kind: '-', text: midA[i], aIdx: prefix + i + 1, prevA: prefix + i, prevB: prefix + j})
	}
	for ; j < m; j++ {
		ops = append(ops, op{kind: '+', text: midB[j], bIdx: prefix + j + 1, prevA: prefix + i, prevB: prefix + j})
	}

	suffixStartA := len(aLines) - suffix
	suffixStartB := len(bLines) - suffix
	for k := 0; k < sContext; k++ {
		idxA := suffixStartA + k
		idxB := suffixStartB + k
		ops = append(ops, op{
			kind:  ' ',
			text:  aLines[idxA],
			aIdx:  idxA + 1,
			bIdx:  idxB + 1,
			prevA: idxA,
			prevB: idxB,
		})
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

func degradedDiff(midA, midB []string, prefix, n, m int) string {
	startA := prefix + 1
	if n == 0 {
		startA = prefix
	}
	startB := prefix + 1
	if m == 0 {
		startB = prefix
	}
	header := fmt.Sprintf("@@ -%s +%s @@ (diff too large: %d lines removed, %d lines added)",
		formatRange(startA, n), formatRange(startB, m), n, m)
	out := []string{header}
	out = append(out, sampleLines(midA, '-')...)
	out = append(out, sampleLines(midB, '+')...)
	return strings.Join(out, "\n")
}

func sampleLines(lines []string, prefix byte) []string {
	const sampleSize = 3
	if len(lines) <= sampleSize*2 {
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = fmt.Sprintf("%c%s", prefix, l)
		}
		return out
	}
	out := make([]string, 0, sampleSize*2+1)
	for i := 0; i < sampleSize; i++ {
		out = append(out, fmt.Sprintf("%c%s", prefix, lines[i]))
	}
	out = append(out, "...")
	for i := len(lines) - sampleSize; i < len(lines); i++ {
		out = append(out, fmt.Sprintf("%c%s", prefix, lines[i]))
	}
	return out
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
