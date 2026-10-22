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
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/baseline"
)

// BaselineEntry represents a single tolerated dropped error site in a baseline file.
type BaselineEntry = baseline.Entry

// ParseBaseline parses baseline entries from raw bytes.
var ParseBaseline = baseline.ParseBaseline

// FormatCategoryKey formats category and key.
var FormatCategoryKey = baseline.FormatCategoryKey

// LoadBaseline loads baseline entries from the given path.
var LoadBaseline = baseline.LoadBaseline

// FormatBaseline formats baseline entries into a sorted, human-readable string.
func FormatBaseline(entries []BaselineEntry) string {
	return baseline.FormatBaseline("droppederrors", entries)
}

// WriteBaseline writes baseline entries to the given path, creating any missing parent directories.
func WriteBaseline(path string, entries []BaselineEntry) error {
	return baseline.WriteBaseline(path, "droppederrors", entries)
}

// MatchFindings compares findings against baseline entries using multiset matching.
func MatchFindings(findings []Finding, baselineEntries []BaselineEntry, checkedFiles map[string]bool, rootDir string) (newFindings []Finding, staleEntries []BaselineEntry) {
	bFindings := make([]baseline.Finding, len(findings))
	for i, f := range findings {
		bFindings[i] = baseline.Finding{
			Pos:           f.Pos,
			Category:      f.Category,
			Key:           f.Callee,
			Callee:        f.Callee,
			EnclosingFunc: f.EnclosingFunc,
			RelFile:       f.RelFile,
			RepoRel:       f.RepoRel,
			ModRel:        f.ModRel,
		}
	}
	newBFindings, staleEntries := baseline.MatchFindings(bFindings, baselineEntries, checkedFiles, rootDir, "droppederrors")
	newFindings = make([]Finding, len(newBFindings))
	for i, bf := range newBFindings {
		newFindings[i] = Finding{
			Pos:           bf.Pos,
			Category:      bf.Category,
			Callee:        bf.Key,
			EnclosingFunc: bf.EnclosingFunc,
			RelFile:       bf.RelFile,
			RepoRel:       bf.RepoRel,
			ModRel:        bf.ModRel,
		}
	}
	return newFindings, staleEntries
}
