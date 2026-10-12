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

package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelloHandler_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	helloHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Body.String(); got != "Hello world" {
		t.Errorf("expected body %q, got %q", "Hello world", got)
	}
}

type errResponseWriter struct {
	header http.Header
}

func (e *errResponseWriter) Header() http.Header {
	if e.header == nil {
		e.header = make(http.Header)
	}
	return e.header
}

func (e *errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func (e *errResponseWriter) WriteHeader(statusCode int) {}

func TestHelloHandler_WriteError(t *testing.T) {
	var buf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(origOutput)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := &errResponseWriter{}

	helloHandler(w, req)

	logged := buf.String()
	if !strings.Contains(logged, "writing response: simulated write error") {
		t.Errorf("expected log to contain write error, got %q", logged)
	}
}
