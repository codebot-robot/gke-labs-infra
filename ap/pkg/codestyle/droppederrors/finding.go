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

const (
	CategoryDroppedError               = baseline.CategoryDroppedError
	CategoryErrorOnlyCheckedForSuccess = baseline.CategoryErrorOnlyCheckedForSuccess
	CategoryErrorDiscarded             = baseline.CategoryErrorDiscarded
)

// Finding represents an unchecked error finding in source code.
type Finding = baseline.Finding

// BaselineEntry represents a baseline entry.
type BaselineEntry = baseline.Entry
