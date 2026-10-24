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

package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownload_Success(t *testing.T) {
	content := []byte("download test content")
	hasher := sha256.New()
	hasher.Write(content)
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	path, err := Download(t.Context(), server.URL, digest, tempDir)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	defer func() {
		_ = os.Remove(path)
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestDownload_DigestMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bad content"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	_, err := Download(t.Context(), server.URL, "sha256:0000000000000000000000000000000000000000000000000000000000000000", tempDir)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected error to mention checksum mismatch, got %v", err)
	}
}

func TestDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	_, err := Download(t.Context(), server.URL, "", tempDir)
	if err == nil {
		t.Fatal("expected HTTP error, got nil")
	}
}

func TestDigest(t *testing.T) {
	content := []byte("digest test content")
	hasher := sha256.New()
	hasher.Write(content)
	expectedDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	d, err := Digest(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	if d != expectedDigest {
		t.Errorf("got digest %q, want %q", d, expectedDigest)
	}
}
