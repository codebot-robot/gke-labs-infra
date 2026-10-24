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

package kind

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/scoped"
	"k8s.io/klog/v2"
)

type failTransport struct{}

func (f *failTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network is disabled: %s", req.URL)
}

func TestEnsureKind_Resolution(t *testing.T) {
	ctx := scoped.WithHTTPClient(t.Context(), &http.Client{Transport: &failTransport{}})

	// 1. "latest" resolves to table entry with no network
	ver, downloadURL, digest, err := resolveKindRelease(ctx, "latest")
	if err != nil {
		t.Fatalf("resolveKindRelease(latest) failed: %v", err)
	}
	latestVer := Latest().Version
	if ver != latestVer {
		t.Errorf("expected version %s, got %s", latestVer, ver)
	}
	if !strings.Contains(downloadURL, latestVer) {
		t.Errorf("expected download URL to contain %s, got %s", latestVer, downloadURL)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("expected digest with sha256: prefix, got %s", digest)
	}

	// 2. Listed version (v0.32.0) resolves to table entry with no network
	ver, downloadURL, digest, err = resolveKindRelease(ctx, "v0.32.0")
	if err != nil {
		t.Fatalf("resolveKindRelease(v0.32.0) failed: %v", err)
	}
	if ver != "v0.32.0" {
		t.Errorf("expected version v0.32.0, got %s", ver)
	}
	if !strings.Contains(downloadURL, "v0.32.0") {
		t.Errorf("expected download URL to contain v0.32.0, got %s", downloadURL)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("expected digest with sha256: prefix, got %s", digest)
	}

	// 3. Unlisted version (v0.25.0) warns and falls back to GitHub API (which fails via failing client)
	var logBuf bytes.Buffer
	klog.SetOutput(&logBuf)
	klog.LogToStderr(false)
	defer func() {
		klog.SetOutput(os.Stderr)
		klog.LogToStderr(true)
	}()

	_, _, _, err = resolveKindRelease(ctx, "v0.25.0")
	if err == nil {
		t.Fatal("expected error from failed network request on unlisted version, got nil")
	}
	if !strings.Contains(err.Error(), "network is disabled") {
		t.Errorf("expected error to mention disabled network, got: %v", err)
	}

	wantWarn := "kind v0.25.0 is not in versions.yaml; resolving it from the GitHub API, which is rate limited; run 'ap tool update-versions' to add it"
	if !strings.Contains(logBuf.String(), wantWarn) {
		t.Errorf("expected warning %q in log output, got: %s", wantWarn, logBuf.String())
	}
}

func TestColdRun_DoesNotMutateHome(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	kindBinContent := []byte("#!/bin/sh\necho kind v0.33.0\n")
	hasher := sha256.New()
	hasher.Write(kindBinContent)
	kindDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(kindBinContent)
	}))
	defer server.Close()

	t.Setenv("KIND_DOWNLOAD_BASE_URL", server.URL)
	t.Setenv("KIND_DOWNLOAD_DIGEST", kindDigest)

	binPath, err := EnsureKind(t.Context(), "v0.33.0")
	if err != nil {
		t.Fatalf("EnsureKind failed: %v", err)
	}
	if !fileutils.IsExecutable(binPath) {
		t.Errorf("expected kind binary %s to be executable", binPath)
	}

	entries, err := os.ReadDir(tempHome)
	if err != nil {
		t.Fatalf("failed to read temp home: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected HOME (%s) to remain untouched, but found %d entries: %v", tempHome, len(entries), entries)
	}
}
