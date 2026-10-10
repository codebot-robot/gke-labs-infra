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
	"bytes"
	"errors"
	"testing"
)

func TestPrinterSuccess(t *testing.T) {
	var buf bytes.Buffer
	p := newPrinter(&buf)

	p.Printf("hello %s\n", "world")
	p.Println("a", "b")
	p.Print("c", "d\n")

	if err := p.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "hello world\na b\ncd\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type errWriter struct {
	writesAllowed int
	err           error
	calls         int
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	w.calls++
	if w.writesAllowed > 0 {
		w.writesAllowed--
		return len(p), nil
	}
	return 0, w.err
}

func TestPrinterErrorAccumulation(t *testing.T) {
	expectedErr := errors.New("simulated write error")
	w := &errWriter{
		writesAllowed: 1,
		err:           expectedErr,
	}
	p := newPrinter(w)

	p.Printf("first call succeeds: %d\n", 1)
	if err := p.Err(); err != nil {
		t.Fatalf("unexpected error after first write: %v", err)
	}

	// This call triggers the error
	p.Printf("second call fails\n")
	if !errors.Is(p.Err(), expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, p.Err())
	}

	// Further calls should be no-ops and not attempt writes to the underlying writer
	callCount := w.calls
	p.Printf("third call should be no-op\n")
	p.Println("fourth call should be no-op")
	p.Print("fifth call should be no-op")

	if w.calls != callCount {
		t.Fatalf("expected writer calls to stay %d, got %d", callCount, w.calls)
	}
	if !errors.Is(p.Err(), expectedErr) {
		t.Fatalf("expected error to remain %v, got %v", expectedErr, p.Err())
	}
}

func TestPrinterNilHandling(t *testing.T) {
	var p *printer
	if err := p.Err(); err != nil {
		t.Fatalf("expected nil error for nil printer, got %v", err)
	}

	// Calling methods on nil printer should not panic
	p.Printf("test %s\n", "format")
	p.Println("test")
	p.Print("test")

	pDefault := newPrinter(nil)
	if pDefault.w == nil {
		t.Fatal("expected non-nil default writer")
	}
}

func TestPrintIndentedWithPrinter(t *testing.T) {
	var buf bytes.Buffer
	p := newPrinter(&buf)

	lines := []string{"first line", "second line"}
	printIndented(p, "  ", lines)

	if err := p.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "  first line\n  second line\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintIndentedWriterError(t *testing.T) {
	expectedErr := errors.New("write failure")
	w := &errWriter{
		writesAllowed: 0,
		err:           expectedErr,
	}
	p := newPrinter(w)

	lines := []string{"first line", "second line"}
	printIndented(p, "  ", lines)

	if !errors.Is(p.Err(), expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, p.Err())
	}
	if w.calls != 1 {
		t.Fatalf("expected exactly 1 call (short-circuited after failure), got %d", w.calls)
	}
}
