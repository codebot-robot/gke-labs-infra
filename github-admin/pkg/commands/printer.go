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

package commands

import (
	"fmt"
	"io"
	"os"
)

// printer wraps an io.Writer and records the first write error encountered.
// Once an error occurs, subsequent write operations are no-ops.
type printer struct {
	w   io.Writer
	err error
}

func newPrinter(w io.Writer) *printer {
	if w == nil {
		w = os.Stdout
	}
	return &printer{w: w}
}

// Printf writes formatted text to the underlying writer.
// If an error previously occurred, the write is skipped.
func (p *printer) Printf(format string, args ...any) {
	if p == nil || p.err != nil {
		return
	}
	if _, err := fmt.Fprintf(p.w, format, args...); err != nil {
		p.err = err
	}
}

// Println writes operands followed by a newline to the underlying writer.
// If an error previously occurred, the write is skipped.
func (p *printer) Println(args ...any) {
	if p == nil || p.err != nil {
		return
	}
	if _, err := fmt.Fprintln(p.w, args...); err != nil {
		p.err = err
	}
}

// Print writes operands to the underlying writer.
// If an error previously occurred, the write is skipped.
func (p *printer) Print(args ...any) {
	if p == nil || p.err != nil {
		return
	}
	if _, err := fmt.Fprint(p.w, args...); err != nil {
		p.err = err
	}
}

// Err returns the first write error encountered, or nil if no error occurred.
func (p *printer) Err() error {
	if p == nil {
		return nil
	}
	return p.err
}
