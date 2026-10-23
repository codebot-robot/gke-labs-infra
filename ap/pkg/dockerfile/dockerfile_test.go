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
	"reflect"
	"testing"
)

func TestFromLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []From
	}{
		{
			name:    "--platform=...",
			content: "FROM --platform=$BUILDPLATFORM golang:1.25\n",
			want: []From{
				{
					Line:  1,
					Flags: []string{"--platform=$BUILDPLATFORM"},
					Image: "golang:1.25",
				},
			},
		},
		{
			name:    "lowercase from",
			content: "from golang:1.25\n",
			want: []From{
				{
					Line:  1,
					Image: "golang:1.25",
				},
			},
		},
		{
			name:    "AS builder",
			content: "FROM golang:1.25 AS builder\n",
			want: []From{
				{
					Line:  1,
					Image: "golang:1.25",
					Alias: "builder",
				},
			},
		},
		{
			name: "non-FROM line containing golang:",
			content: `# see golang:1.20 notes
RUN echo golang:1.26
FROM golang:1.25
`,
			want: []From{
				{
					Line:  3,
					Image: "golang:1.25",
				},
			},
		},
		{
			name: "multi-stage with flags, lowercase from, and alias",
			content: `# Build stage
from --platform=linux/amd64 golang:1.25-alpine as build
RUN go build .
# Runtime stage
FROM alpine:3.20 AS runner
COPY --from=build /app /app
`,
			want: []From{
				{
					Line:  2,
					Flags: []string{"--platform=linux/amd64"},
					Image: "golang:1.25-alpine",
					Alias: "build",
				},
				{
					Line:  5,
					Image: "alpine:3.20",
					Alias: "runner",
				},
			},
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromLines(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromLines() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
