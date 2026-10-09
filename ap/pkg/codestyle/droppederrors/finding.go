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

import "go/token"

const (
	CategoryDroppedError               = "dropped-error"
	CategoryErrorOnlyCheckedForSuccess = "error-only-checked-for-success"
)

// Finding represents an unchecked error finding in source code.
type Finding struct {
	Pos           token.Position
	Category      string // CategoryDroppedError or CategoryErrorOnlyCheckedForSuccess
	Callee        string
	EnclosingFunc string
	RelFile       string // Relative to AP root
	RepoRel       string // Relative to repo root
	ModRel        string // Relative to module dir
}

// Key returns a unique signature for deduplication.
func (f Finding) Key() string {
	return f.Pos.Filename + ":" + f.Pos.String() + ":" + f.Category + ":" + f.EnclosingFunc + ":" + f.Callee
}
