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
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BaselineEntry represents a single tolerated dropped error site in a baseline file.
type BaselineEntry struct {
	File          string
	EnclosingFunc string
	Category      string
	Callee        string
}

// String returns the canonical string representation of a baseline entry.
func (e BaselineEntry) String() string {
	if e.Category != "" && e.Category != CategoryDroppedError {
		return fmt.Sprintf("%s:%s:%s:%s", e.File, e.EnclosingFunc, e.Category, e.Callee)
	}
	return fmt.Sprintf("%s:%s:%s", e.File, e.EnclosingFunc, e.Callee)
}

// ParseBaseline parses baseline entries from raw bytes.
// Comments (starting with '#') and empty lines are ignored.
// Entries may be formatted as:
// - "file:enclosing:callee"
// - "file:enclosing:category:callee"
// - or whitespace-separated equivalents.
func ParseBaseline(data []byte) ([]BaselineEntry, error) {
	var entries []BaselineEntry
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
			entries = append(entries, BaselineEntry{
				File:          filepath.ToSlash(fields[0]),
				EnclosingFunc: fields[1],
				Category:      fields[2],
				Callee:        fields[3],
			})
			continue
		}
		if len(fields) == 3 {
			entries = append(entries, BaselineEntry{
				File:          filepath.ToSlash(fields[0]),
				EnclosingFunc: fields[1],
				Category:      CategoryDroppedError,
				Callee:        fields[2],
			})
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) == 4 {
			entries = append(entries, BaselineEntry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      parts[2],
				Callee:        parts[3],
			})
			continue
		}
		if len(parts) == 3 {
			entries = append(entries, BaselineEntry{
				File:          filepath.ToSlash(parts[0]),
				EnclosingFunc: parts[1],
				Category:      CategoryDroppedError,
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
func FormatBaseline(entries []BaselineEntry) string {
	sorted := make([]BaselineEntry, len(entries))
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
		return sorted[i].Callee < sorted[j].Callee
	})

	var sb strings.Builder
	sb.WriteString("# Baseline for ap droppederrors ratchet.\n")
	sb.WriteString("# Entries: <file>:<enclosing-function>[:<category>]:<callee>\n")
	sb.WriteString("# Do not add entries manually; fix errors or run 'ap lint droppederrors --write-baseline' if approved.\n")
	for _, e := range sorted {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// LoadBaseline loads baseline entries from the given path.
// If the file does not exist, it returns an empty slice and nil error.
func LoadBaseline(path string) ([]BaselineEntry, error) {
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
func WriteBaseline(path string, entries []BaselineEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	content := FormatBaseline(entries)
	return os.WriteFile(path, []byte(content), 0644)
}

// MatchFindings compares findings against baseline entries using multiset matching.
// Returns findings not in the baseline (newFindings) and baseline entries that no longer occur
// in checked files (staleEntries).
func MatchFindings(findings []Finding, baseline []BaselineEntry, checkedFiles map[string]bool, rootDir string) (newFindings []Finding, staleEntries []BaselineEntry) {
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

func matchesEntry(entry BaselineEntry, f Finding) bool {
	if entry.Callee != f.Callee || entry.EnclosingFunc != f.EnclosingFunc {
		return false
	}

	// Normalize category comparison
	entryCat := entry.Category
	if entryCat == "" {
		entryCat = CategoryDroppedError
	}
	fCat := f.Category
	if fCat == "" {
		fCat = CategoryDroppedError
	}
	if entryCat != fCat {
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

func isStaleCandidate(entry BaselineEntry, checkedFiles map[string]bool, rootDir string) bool {
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
