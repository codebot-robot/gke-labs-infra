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

// Entry represents a single tolerated finding in a baseline file.
type Entry struct {
	File          string
	EnclosingFunc string
	Category      string
	Key           string
	Callee        string // alias for Key, populated for backward compatibility with droppederrors
}

// String returns the canonical string representation of a baseline entry.
func (e Entry) String() string {
	key := e.Key
	if key == "" {
		key = e.Callee
	}
	if e.Category != "" && e.Category != "dropped-error" && e.Category != "droppederrors" {
		return fmt.Sprintf("%s:%s:%s:%s", e.File, e.EnclosingFunc, e.Category, key)
	}
	return fmt.Sprintf("%s:%s:%s", e.File, e.EnclosingFunc, key)
}

// ParseBaseline parses baseline entries from raw bytes.
// Comments (starting with '#') and empty lines are ignored.
// Entries may be formatted as:
// - "file:enclosing:key"
// - "file:enclosing:category:key"
// - or whitespace-separated equivalents.
func ParseBaseline(data []byte) ([]Entry, error) {
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
				Key:           fields[3],
				Callee:        fields[3],
			})
			continue
		}
		if len(fields) == 3 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(fields[0]),
				EnclosingFunc: fields[1],
				Category:      "dropped-error",
				Key:           fields[2],
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
				Key:           parts[3],
				Callee:        parts[3],
			})
			continue
		}
		if len(parts) == 3 {
			entries = append(entries, Entry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      "dropped-error",
				Key:           parts[2],
				Callee:        parts[2],
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

// FormatBaseline formats baseline entries into a sorted, human-readable string.
func FormatBaseline(checkName string, entries []Entry) string {
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
		keyI := sorted[i].Key
		if keyI == "" {
			keyI = sorted[i].Callee
		}
		keyJ := sorted[j].Key
		if keyJ == "" {
			keyJ = sorted[j].Callee
		}
		return keyI < keyJ
	})

	var sb strings.Builder
	if checkName == "droppederrors" {
		sb.WriteString("# Baseline for ap droppederrors ratchet.\n")
		sb.WriteString("# Entries: <file>:<enclosing-function>[:<category>]:<callee>\n")
		sb.WriteString("# Do not add entries manually; fix errors or run 'ap lint droppederrors --write-baseline' if approved.\n")
	} else {
		sb.WriteString(fmt.Sprintf("# Baseline for ap %s ratchet.\n", checkName))
		sb.WriteString("# Entries: <file>:<enclosing-function>[:<category>]:<key>\n")
		sb.WriteString(fmt.Sprintf("# Do not add entries manually; fix errors or run 'ap lint %s --write-baseline' if approved.\n", checkName))
	}
	for _, e := range sorted {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// LoadBaseline loads baseline entries from the given path.
// If the file does not exist, it returns an empty slice and nil error.
func LoadBaseline(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read baseline %s: %w", path, err)
	}
	return ParseBaseline(data)
}

// WriteBaseline writes baseline entries to the given path, creating any missing parent directories.
func WriteBaseline(path string, checkName string, entries []Entry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	content := FormatBaseline(checkName, entries)
	return os.WriteFile(path, []byte(content), 0644)
}

// Finding represents a diagnostic finding in source code.
type Finding struct {
	Pos           token.Position
	Category      string
	Key           string
	Callee        string // alias for Key
	EnclosingFunc string
	RelFile       string // Relative to AP root
	RepoRel       string // Relative to repo root
	ModRel        string // Relative to module dir
	Message       string
	Check         string
}

// FindingKey returns a unique signature for deduplication.
func (f Finding) FindingKey() string {
	k := f.Key
	if k == "" {
		k = f.Callee
	}
	return f.Pos.Filename + ":" + f.Pos.String() + ":" + f.Category + ":" + f.EnclosingFunc + ":" + k
}

// MatchFindings compares findings against baseline entries using multiset matching.
// Returns findings not in the baseline (newFindings) and baseline entries that no longer occur
// in checked files (staleEntries).
func MatchFindings(findings []Finding, baseline []Entry, checkedFiles map[string]bool, rootDir string, checkName string) (newFindings []Finding, staleEntries []Entry) {
	usedBaseline := make([]bool, len(baseline))

	for _, f := range findings {
		matched := false
		for i, entry := range baseline {
			if usedBaseline[i] {
				continue
			}
			if matchesEntry(entry, f, checkName) {
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

func matchesEntry(entry Entry, f Finding, checkName string) bool {
	entryKey := entry.Key
	if entryKey == "" {
		entryKey = entry.Callee
	}
	fKey := f.Key
	if fKey == "" {
		fKey = f.Callee
	}
	if entryKey != fKey || entry.EnclosingFunc != f.EnclosingFunc {
		return false
	}

	if !matchesCategory(entry.Category, f.Category, checkName) {
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

func matchesCategory(entryCat, fCat, checkName string) bool {
	if entryCat == fCat {
		return true
	}
	// For droppederrors: dropped-error and empty are equivalent
	if (checkName == "droppederrors" || checkName == "") && (entryCat == "" || entryCat == "dropped-error") && (fCat == "" || fCat == "dropped-error") {
		return true
	}
	// For any check: if entry has no category or category equals check name, and finding has check name or empty category
	if (entryCat == "" || entryCat == checkName) && (fCat == "" || fCat == checkName) {
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

// FormatCategoryKey encodes a category and stable key into a category string with structured suffix.
func FormatCategoryKey(category, key string) string {
	if key == "" {
		return category
	}
	return category + ";key=" + key
}

// ParseCategoryKey decodes a category and stable key from a category string.
func ParseCategoryKey(cat string) (category, key string) {
	if c, k, ok := strings.Cut(cat, ";key="); ok {
		return c, k
	}
	return cat, ""
}

// FindEnclosingFunc returns the enclosing function or method name for the given AST position.
// If outside of any function or method, it returns "<init>".
func FindEnclosingFunc(file *ast.File, fset *token.FileSet, pos token.Pos) string {
	if file == nil || fset == nil || !pos.IsValid() {
		return "<init>"
	}
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

// FormatEnclosingFunc formats a function or method declaration name.
func FormatEnclosingFunc(fn *ast.FuncDecl) string {
	if fn == nil {
		return "<init>"
	}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			return fmt.Sprintf("(*%s).%s", types.ExprString(star.X), fn.Name.Name)
		}
		return fmt.Sprintf("(%s).%s", types.ExprString(recv), fn.Name.Name)
	}
	return fn.Name.Name
}
