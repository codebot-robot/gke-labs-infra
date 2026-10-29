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

package baseline

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CategoryDroppedError               = "dropped-error"
	CategoryErrorOnlyCheckedForSuccess = "error-only-checked-for-success"
	CategoryErrorDiscarded             = "error-discarded"
)

// Entry represents a single tolerated finding site in a baseline file.
type Entry struct {
	File          string
	EnclosingFunc string
	Category      string
	Detail        string
	Callee        string // Synced with Detail for compatibility
}

// String returns the canonical string representation of a baseline entry.
func (e Entry) String() string {
	detail := e.Detail
	if detail == "" {
		detail = e.Callee
	}
	if e.Category != "" && e.Category != CategoryDroppedError {
		return fmt.Sprintf("%s:%s:%s:%s", e.File, e.EnclosingFunc, e.Category, detail)
	}
	return fmt.Sprintf("%s:%s:%s", e.File, e.EnclosingFunc, detail)
}

// Finding represents a diagnostic or finding in source code.
type Finding struct {
	Pos           token.Position
	Category      string
	Detail        string
	Callee        string // Synced with Detail for compatibility
	EnclosingFunc string
	RelFile       string // Relative to AP root / baseline root
	RepoRel       string // Relative to repo root
	ModRel        string // Relative to module dir
	Message       string
	Related       []RelatedInfo
}

// RelatedInfo represents secondary location information for a finding.
type RelatedInfo struct {
	Pos     token.Position
	Message string
}

// Key returns a unique signature for deduplication.
func (f Finding) Key() string {
	detail := f.Detail
	if detail == "" {
		detail = f.Callee
	}
	return f.Pos.Filename + ":" + f.Pos.String() + ":" + f.Category + ":" + f.EnclosingFunc + ":" + detail
}

// Parse parses baseline entries from raw bytes.
// Comments (starting with '#') and empty lines are ignored.
// Entries may be formatted as:
// - "file:enclosing:detail"
// - "file:enclosing:category:detail"
// - or whitespace-separated equivalents.
func Parse(data []byte) ([]Entry, error) {
	var entries []Entry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 4 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(fields[0]),
				EnclosingFunc: fields[1],
				Category:      fields[2],
				Detail:        fields[3],
				Callee:        fields[3],
			})
			continue
		}
		if len(fields) == 3 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(fields[0]),
				EnclosingFunc: fields[1],
				Category:      "",
				Detail:        fields[2],
				Callee:        fields[2],
			})
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) == 4 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      parts[2],
				Detail:        parts[3],
				Callee:        parts[3],
			})
			continue
		}
		if len(parts) == 3 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      "",
				Detail:        parts[2],
				Callee:        parts[2],
			})
			continue
		}
		if len(parts) > 4 {
			if parts[2] == CategoryErrorOnlyCheckedForSuccess || parts[2] == CategoryDroppedError || parts[2] == CategoryErrorDiscarded {
				entries = append(entries, Entry{
					File:          filepath.ToSlash(parts[0]),
					EnclosingFunc: parts[1],
					Category:      parts[2],
					Detail:        strings.Join(parts[3:], ":"),
					Callee:        strings.Join(parts[3:], ":"),
				})
				continue
			}
			entries = append(entries, Entry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      "",
				Detail:        strings.Join(parts[2:], ":"),
				Callee:        strings.Join(parts[2:], ":"),
			})
			continue
		}

		return nil, fmt.Errorf("line %d: invalid baseline entry %q", lineNum, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading baseline: %w", err)
	}
	return entries, nil
}

// Format formats baseline entries into a sorted, human-readable string.
func Format(analyzer string, entries []Entry) string {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].EnclosingFunc != sorted[j].EnclosingFunc {
			return sorted[i].EnclosingFunc < sorted[j].EnclosingFunc
		}
		if sorted[i].Category != sorted[j].Category {
			return sorted[i].Category < sorted[j].Category
		}
		detailI := sorted[i].Detail
		if detailI == "" {
			detailI = sorted[i].Callee
		}
		detailJ := sorted[j].Detail
		if detailJ == "" {
			detailJ = sorted[j].Callee
		}
		return detailI < detailJ
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Baseline for ap %s ratchet.\n", analyzer))
	if analyzer == "droppederrors" {
		sb.WriteString("# Entries: <file>:<enclosing-function>[:<category>]:<callee>\n")
	} else {
		sb.WriteString("# Entries: <file>:<enclosing-function>[:<category>]:<detail>\n")
	}
	sb.WriteString(fmt.Sprintf("# Do not add entries manually; fix errors or run 'ap lint %s --write-baseline' if approved.\n", analyzer))
	for _, e := range sorted {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Load loads baseline entries from the given path.
// If the file does not exist, it returns an empty slice and nil error.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read baseline %s: %w", path, err)
	}
	return Parse(data)
}

// Write writes baseline entries to the given path, creating any missing parent directories.
func Write(path, analyzer string, entries []Entry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	content := Format(analyzer, entries)
	return os.WriteFile(path, []byte(content), 0644)
}

// Match compares findings against baseline entries using multiset matching.
// Returns findings not in the baseline (newFindings) and baseline entries that no longer occur
// in checked files (staleEntries).
func Match(findings []Finding, baseline []Entry, checkedFiles map[string]bool, rootDir string) (newFindings []Finding, staleEntries []Entry) {
	usedBaseline := make([]bool, len(baseline))

	for _, f := range findings {
		matched := false
		for i, entry := range baseline {
			if usedBaseline[i] {
				continue
			}
			if matchesEntry(entry, f) {
				usedBaseline[i] = true
				matched = true
				break
			}
		}
		if !matched {
			newFindings = append(newFindings, f)
		}
	}

	for i, entry := range baseline {
		if usedBaseline[i] {
			continue
		}
		if isStaleCandidate(entry, checkedFiles, rootDir) {
			staleEntries = append(staleEntries, entry)
		}
	}

	return newFindings, staleEntries
}

func normalizeCategory(cat string) string {
	if cat == CategoryDroppedError {
		return ""
	}
	return cat
}

func matchesEntry(entry Entry, f Finding) bool {
	entryDetail := entry.Detail
	if entryDetail == "" {
		entryDetail = entry.Callee
	}
	fDetail := f.Detail
	if fDetail == "" {
		fDetail = f.Callee
	}
	if entryDetail != fDetail || entry.EnclosingFunc != f.EnclosingFunc {
		return false
	}

	if normalizeCategory(entry.Category) != normalizeCategory(f.Category) {
		return false
	}

	entryFile := filepath.ToSlash(entry.File)
	if entryFile == filepath.ToSlash(f.RelFile) || entryFile == filepath.ToSlash(f.RepoRel) || entryFile == filepath.ToSlash(f.ModRel) || entryFile == filepath.ToSlash(f.Pos.Filename) {
		return true
	}
	if strings.HasSuffix(filepath.ToSlash(f.Pos.Filename), "/"+entryFile) {
		return true
	}
	return false
}

func isStaleCandidate(entry Entry, checkedFiles map[string]bool, rootDir string) bool {
	entryFile := filepath.ToSlash(entry.File)
	if checkedFiles != nil {
		if checkedFiles[entryFile] {
			return true
		}
	}
	if rootDir != "" {
		fullPath := filepath.Join(rootDir, filepath.FromSlash(entryFile))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// ResolveBaselinePath determines the baseline file path given repoRoot, module dir,
// analyzer name, any explicitly configured path (from CLI or config), and whether write-baseline is requested.
func ResolveBaselinePath(repoRoot, dir, analyzer, configured string, writeBaseline bool) string {
	if configured != "" {
		if filepath.IsAbs(configured) {
			return configured
		}
		if repoRoot != "" {
			return filepath.Join(repoRoot, configured)
		}
		return filepath.Join(dir, configured)
	}

	root := repoRoot
	if root == "" {
		root = dir
	}

	defaultFilename := fmt.Sprintf("%s-baseline.txt", analyzer)
	defaultBaseline := filepath.Join(root, ".ap", defaultFilename)
	if fi, err := os.Stat(defaultBaseline); err == nil && !fi.IsDir() {
		return defaultBaseline
	}

	if dir != "" && dir != root {
		modBaseline := filepath.Join(dir, ".ap", defaultFilename)
		if fi, err := os.Stat(modBaseline); err == nil && !fi.IsDir() {
			return modBaseline
		}
	}

	if writeBaseline {
		return defaultBaseline
	}
	return ""
}

// FindEnclosingFunc returns the enclosing function or method name for pos in file.
func FindEnclosingFunc(file *ast.File, fset *token.FileSet, pos token.Pos) string {
	targetOffset := fset.Position(pos).Offset

	// Check function declarations first
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			start := fset.Position(fn.Pos()).Offset
			end := fset.Position(fn.End()).Offset
			if targetOffset >= start && targetOffset <= end {
				return FormatEnclosingFunc(fn)
			}
		}
	}

	// Check package-level variable declarations (e.g. var x = func() { _ = f() })
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				if vspec, ok := spec.(*ast.ValueSpec); ok {
					start := fset.Position(vspec.Pos()).Offset
					end := fset.Position(vspec.End()).Offset
					if targetOffset >= start && targetOffset <= end {
						if len(vspec.Names) > 0 {
							return vspec.Names[0].Name
						}
					}
				}
			}
		}
	}

	return "<init>"
}

// FormatEnclosingFunc formats an ast.FuncDecl into a function signature string.
func FormatEnclosingFunc(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			return fmt.Sprintf("(*%s).%s", types.ExprString(star.X), fn.Name.Name)
		}
		return fmt.Sprintf("(%s).%s", types.ExprString(recv), fn.Name.Name)
	}
	return fn.Name.Name
}
