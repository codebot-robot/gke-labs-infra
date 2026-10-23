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

package imageref

import (
	"errors"
	"fmt"
	"strings"
)

// Reference represents a parsed container image reference.
type Reference struct {
	Registry   string // "" when none; "docker.io", "gcr.io", "localhost:5000", ...
	Repository string // "golang", "library/golang", "distroless/static", ...
	Tag        string // "" when none
	Digest     string // "" when none; "sha256:..." otherwise
}

// Parse parses an image reference string into a Reference.
// Parsing order:
// 1. Inputs starting with "${" or "$" return an error so callers can skip templated images.
// 2. Split @digest off the end.
// 3. The tag is what follows the last ":" provided that part contains no "/".
// 4. The first path element is the registry if it contains "." or ":" or equals "localhost".
func Parse(s string) (Reference, error) {
	if s == "" {
		return Reference{}, errors.New("empty image reference")
	}

	if strings.HasPrefix(s, "${") || strings.HasPrefix(s, "$") {
		return Reference{}, fmt.Errorf("templated image reference: %q", s)
	}

	var digest string
	rem := s
	if atIdx := strings.LastIndex(rem, "@"); atIdx != -1 {
		digest = rem[atIdx+1:]
		rem = rem[:atIdx]
	}

	var tag string
	if colonIdx := strings.LastIndex(rem, ":"); colonIdx != -1 {
		candidate := rem[colonIdx+1:]
		if !strings.Contains(candidate, "/") {
			tag = candidate
			rem = rem[:colonIdx]
		}
	}

	var registry string
	repository := rem
	if slashIdx := strings.Index(rem, "/"); slashIdx != -1 {
		firstPart := rem[:slashIdx]
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") || firstPart == "localhost" {
			registry = firstPart
			repository = rem[slashIdx+1:]
		}
	}

	return Reference{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
		Digest:     digest,
	}, nil
}

// String formats the Reference back into its string representation,
// exactly round-tripping the Parse input.
func (r Reference) String() string {
	var b strings.Builder
	if r.Registry != "" {
		b.WriteString(r.Registry)
		b.WriteString("/")
	}
	b.WriteString(r.Repository)
	if r.Tag != "" {
		b.WriteString(":")
		b.WriteString(r.Tag)
	}
	if r.Digest != "" {
		b.WriteString("@")
		b.WriteString(r.Digest)
	}
	return b.String()
}

// Name returns the last path element of Repository, e.g. "golang".
func (r Reference) Name() string {
	if idx := strings.LastIndex(r.Repository, "/"); idx != -1 {
		return r.Repository[idx+1:]
	}
	return r.Repository
}
