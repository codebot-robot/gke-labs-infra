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

package toml

import (
	"strings"
)

// Section represents a TOML section, starting with an optional table header line
// and containing zero or more body lines.
type Section struct {
	HeaderLine string
	Parts      []string
	Lines      []string
}

// Render returns all lines of the Section (header line if present, followed by body lines).
func (s *Section) Render() []string {
	var res []string
	if s.HeaderLine != "" {
		res = append(res, s.HeaderLine)
	}
	res = append(res, s.Lines...)
	return res
}

// ParseTableHeader parses a TOML table header line (e.g. `[plugins."io.containerd.grpc.v1.cri".registry]`)
// into its canonical key parts (e.g. `["plugins", "io.containerd.grpc.v1.cri", "registry"]`).
// It normalizes single and double quotes, handles dot separators, strips whitespace, and ignores
// array-of-tables headers ([[...]]) and trailing comments.
func ParseTableHeader(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	if strings.HasPrefix(trimmed, "[[") {
		// Array of tables, not a standard table header
		return nil, false
	}
	closeIdx := strings.LastIndex(trimmed, "]")
	if closeIdx == -1 || closeIdx == 0 {
		return nil, false
	}
	afterClose := strings.TrimSpace(trimmed[closeIdx+1:])
	if afterClose != "" && !strings.HasPrefix(afterClose, "#") {
		return nil, false
	}
	inner := strings.TrimSpace(trimmed[1:closeIdx])
	var parts []string
	i := 0
	for i < len(inner) {
		for i < len(inner) && (inner[i] == ' ' || inner[i] == '\t') {
			i++
		}
		if i >= len(inner) {
			break
		}
		if inner[i] == '"' || inner[i] == '\'' {
			quote := inner[i]
			i++
			var sb strings.Builder
			for i < len(inner) && inner[i] != quote {
				if quote == '"' && inner[i] == '\\' && i+1 < len(inner) {
					i++
					sb.WriteByte(inner[i])
				} else {
					sb.WriteByte(inner[i])
				}
				i++
			}
			if i < len(inner) && inner[i] == quote {
				i++
			}
			parts = append(parts, sb.String())
		} else {
			start := i
			for i < len(inner) && inner[i] != '.' && inner[i] != ' ' && inner[i] != '\t' {
				i++
			}
			part := strings.TrimSpace(inner[start:i])
			if part != "" {
				parts = append(parts, part)
			}
		}
		for i < len(inner) && (inner[i] == ' ' || inner[i] == '\t') {
			i++
		}
		if i < len(inner) && inner[i] == '.' {
			i++
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}

// ParseSections breaks a TOML file content into top-level and table Sections.
func ParseSections(content string) []*Section {
	rawLines := strings.Split(content, "\n")
	var sections []*Section
	current := &Section{}
	sections = append(sections, current)

	for _, rawLine := range rawLines {
		line := strings.TrimSuffix(rawLine, "\r")
		if parts, ok := ParseTableHeader(line); ok {
			current = &Section{
				HeaderLine: line,
				Parts:      parts,
			}
			sections = append(sections, current)
		} else {
			current.Lines = append(current.Lines, line)
		}
	}
	return sections
}

// ParseEndpoints extracts string endpoints from lines containing an endpoint array.
func ParseEndpoints(lines []string) []string {
	var endpoints []string
	inEndpoint := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
		if strings.HasPrefix(trimmed, "endpoint") {
			inEndpoint = true
		}
		if inEndpoint {
			i := 0
			for i < len(trimmed) {
				if trimmed[i] == '"' || trimmed[i] == '\'' {
					q := trimmed[i]
					i++
					start := i
					for i < len(trimmed) && trimmed[i] != q {
						if q == '"' && trimmed[i] == '\\' && i+1 < len(trimmed) {
							i++
						}
						i++
					}
					if i < len(trimmed) {
						endpoints = append(endpoints, trimmed[start:i])
						i++
					}
				} else {
					i++
				}
			}
			if strings.Contains(trimmed, "]") {
				inEndpoint = false
			}
		}
	}
	return endpoints
}
