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

package dockerfile

import (
	"strings"
)

// From represents a FROM instruction in a Dockerfile.
type From struct {
	Line  int
	Flags []string
	Image string
	Alias string
}

// FromLines parses FROM instructions from Dockerfile content.
// Instruction matching is case-insensitive on the first field.
// Flags are the leading "--" fields before the image.
// Alias is the token after "AS" (case-insensitive).
// Line continuations and heredocs are out of scope.
func FromLines(content string) []From {
	var froms []From
	lines := strings.Split(content, "\n")
	for i, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if !strings.EqualFold(fields[0], "FROM") {
			continue
		}

		idx := 1
		var flags []string
		for idx < len(fields) && strings.HasPrefix(fields[idx], "--") {
			flags = append(flags, fields[idx])
			idx++
		}
		if idx >= len(fields) {
			continue
		}

		image := fields[idx]
		idx++

		alias := ""
		if idx < len(fields) && strings.EqualFold(fields[idx], "AS") && idx+1 < len(fields) {
			alias = fields[idx+1]
		}

		froms = append(froms, From{
			Line:  i + 1,
			Flags: flags,
			Image: image,
			Alias: alias,
		})
	}
	return froms
}
