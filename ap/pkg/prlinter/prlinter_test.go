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

package prlinter

import (
	"testing"
)

func TestCheckDoubleSpacing(t *testing.T) {
	tests := []struct {
		name    string
		diff    string
		wantErr bool
		wantMsg string
	}{
		{
			name:    "no double spacing",
			diff:    "+++ b/main.go\n+line 1\n+line 2\n+line 3\n+line 4\n",
			wantErr: false,
		},
		{
			name:    "alternating blank lines 8",
			diff:    "+++ b/main.go\n+line 1\n+\n+line 2\n+\n+line 3\n+\n+line 4\n+\n",
			wantErr: true,
			wantMsg: "detected double-spaced code in main.go (8+ alternating blank lines)",
		},
		{
			name:    "error double spacing",
			diff:    "+++ b/main.go\n+err := foo()\n+\n+if err != nil {\n",
			wantErr: true,
			wantMsg: "detected double-spaced code in main.go: blank line between error assignment and if err != nil check",
		},
		{
			name:    "error double spacing multiple assignment",
			diff:    "+++ b/main.go\n+val, err := foo()\n+\n+if err != nil {\n",
			wantErr: true,
			wantMsg: "detected double-spaced code in main.go: blank line between error assignment and if err != nil check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDoubleSpacing(tt.diff)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkDoubleSpacing() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Errorf("checkDoubleSpacing() error message = %v, wantMsg %v", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestIsErrAssign(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple err assign",
			input:    "err := foo()",
			expected: true,
		},
		{
			name:     "multiple assignment with space after comma",
			input:    "val, err := foo()",
			expected: true,
		},
		{
			name:     "multiple assignment without space after comma",
			input:    "val,err := foo()",
			expected: true,
		},
		{
			name:     "three assignments",
			input:    "a, b, err := bar()",
			expected: true,
		},
		{
			name:     "no space before colon-equal",
			input:    "err:= foo()",
			expected: true,
		},
		{
			name:     "no space around colon-equal",
			input:    "err:=foo()",
			expected: true,
		},
		{
			name:     "no space anywhere",
			input:    "val,err:=foo()",
			expected: true,
		},
		{
			name:     "trailing assignment",
			input:    "err :=",
			expected: true,
		},
		{
			name:     "my_err variable with prefix",
			input:    "my_err := foo()",
			expected: false,
		},
		{
			name:     "xerr variable",
			input:    "xerr := foo()",
			expected: false,
		},
		{
			name:     "err_val variable with suffix",
			input:    "err_val := foo()",
			expected: false,
		},
		{
			name:     "equality comparison",
			input:    "err == foo()",
			expected: false,
		},
		{
			name:     "plain assignment",
			input:    "err = foo()",
			expected: false,
		},
		{
			name:     "err not directly before :=",
			input:    "err, val := foo()",
			expected: false,
		},
		{
			name:     "assigned to err",
			input:    "foo := err",
			expected: false,
		},
		{
			name:     "if err != nil check",
			input:    "if err != nil {",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isErrAssign(tt.input)
			if got != tt.expected {
				t.Errorf("isErrAssign(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsIfErrCheck(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "standard if err != nil with brace space",
			input:    "if err != nil {",
			expected: true,
		},
		{
			name:     "extra whitespace",
			input:    "if   err   !=   nil   {",
			expected: true,
		},
		{
			name:     "no space before brace",
			input:    "if err != nil{",
			expected: true,
		},
		{
			name:     "unspaced tokens not matched",
			input:    "if err!=nil{",
			expected: false,
		},
		{
			name:     "if err != nil with inline comment",
			input:    "if err != nil { // handle error",
			expected: true,
		},
		{
			name:     "err == nil",
			input:    "if err == nil {",
			expected: false,
		},
		{
			name:     "myerr variable",
			input:    "if myerr != nil {",
			expected: false,
		},
		{
			name:     "missing brace",
			input:    "if err != nil",
			expected: false,
		},
		{
			name:     "statement without if",
			input:    "err != nil {",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIfErrCheck(tt.input)
			if got != tt.expected {
				t.Errorf("isIfErrCheck(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
