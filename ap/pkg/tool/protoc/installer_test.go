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

package protoc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveProtocAsset(t *testing.T) {
	tests := []struct {
		version string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"36.2", "linux", "amd64", "protoc-36.2-linux-x86_64.zip", false},
		{"v36.2", "linux", "arm64", "protoc-36.2-linux-aarch_64.zip", false},
		{"36.2", "darwin", "amd64", "protoc-36.2-osx-x86_64.zip", false},
		{"36.2", "darwin", "arm64", "protoc-36.2-osx-aarch_64.zip", false},
		{"36.2", "windows", "amd64", "protoc-36.2-win64.zip", false},
		{"36.2", "unknown", "amd64", "", true},
		{"36.2", "linux", "unknown", "", true},
	}

	for _, tt := range tests {
		got, err := ResolveProtocAsset(tt.version, tt.goos, tt.goarch)
		if (err != nil) != tt.wantErr {
			t.Errorf("ResolveProtocAsset(%s, %s, %s) error = %v, wantErr %v", tt.version, tt.goos, tt.goarch, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ResolveProtocAsset(%s, %s, %s) = %s, want %s", tt.version, tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestEnsureProtoc_Cached(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	version := "36.2"
	binDir := filepath.Join(tempCache, "tools", "protoc", version, "bin")
	includeDir := filepath.Join(tempCache, "tools", "protoc", version, "include")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(includeDir, 0755); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(binDir, "protoc")
	if runtime.GOOS == "windows" {
		fakeBin += ".exe"
	}
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho fake\n"), 0755); err != nil {
		t.Fatal(err)
	}

	bin, inc, err := EnsureProtoc(t.Context(), version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != fakeBin {
		t.Errorf("expected bin %s, got %s", fakeBin, bin)
	}
	if inc != includeDir {
		t.Errorf("expected inc %s, got %s", includeDir, inc)
	}
}

func TestEnsureProtoc_OfflineFallback(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	// Pre-populate cache with version 35.1 and 36.2
	for _, v := range []string{"35.1", "36.2"} {
		binDir := filepath.Join(tempCache, "tools", "protoc", v, "bin")
		_ = os.MkdirAll(binDir, 0755)
		fakeBin := filepath.Join(binDir, "protoc")
		if runtime.GOOS == "windows" {
			fakeBin += ".exe"
		}
		_ = os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755)
	}

	// Broken server for GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	// Calling with "latest" when offline should fall back to newest cached (36.2)
	bin, _, err := EnsureProtoc(t.Context(), "latest")
	if err != nil {
		t.Fatalf("unexpected error during offline fallback: %v", err)
	}
	if !strings.Contains(bin, "36.2") {
		t.Errorf("expected fallback to 36.2, got: %s", bin)
	}
}

func TestEnsureProtoc_ChecksumMismatch(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("corrupt zip file"))
	}))
	defer server.Close()

	t.Setenv("PROTOC_DOWNLOAD_BASE_URL", server.URL)
	t.Setenv("PROTOC_DOWNLOAD_DIGEST", "sha256:0000000000000000000000000000000000000000000000000000000000000000")

	_, _, err := EnsureProtoc(t.Context(), "36.2")
	if err == nil {
		t.Fatal("expected error on checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected error to mention checksum mismatch, got: %v", err)
	}
}

func TestDetectRequiredPlugins(t *testing.T) {
	args := []string{
		"-I", "proto",
		"--go_out=paths=source_relative:.",
		"--go_opt=module=github.com/example",
		"--go-grpc_out=.",
		"--python_out=.",
		"--plugin=protoc-gen-custom=/usr/bin/custom",
		"--plugin=protoc-gen-extra",
		"proto/foo.proto",
	}

	got := DetectRequiredPlugins(args)
	want := []string{"protoc-gen-extra", "protoc-gen-go", "protoc-gen-go-grpc"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("DetectRequiredPlugins() = %v, want %v", got, want)
	}
}

func TestEnsurePlugin_Unknown(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	_, err := EnsurePlugin(t.Context(), "protoc-gen-unknown", "latest", nil)
	if err == nil {
		t.Fatal("expected error for unknown plugin, got nil")
	}
	if !strings.Contains(err.Error(), "unknown protoc plugin") || !strings.Contains(err.Error(), ".ap/proto.yaml") {
		t.Errorf("expected helpful declaration error, got: %v", err)
	}
}

func TestEnsurePlugin_Cached(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	pluginDir := filepath.Join(tempCache, "tools", "plugins", "protoc-gen-go", "v1.36.12", "bin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(pluginDir, "protoc-gen-go")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	dir, err := EnsurePlugin(t.Context(), "protoc-gen-go", "v1.36.12", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != pluginDir {
		t.Errorf("expected dir %s, got %s", pluginDir, dir)
	}
}

func TestEnsureProtoc_RealDownloadAndExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping download test in short mode")
	}

	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	ctx := t.Context()
	protocBin, includeDir, err := EnsureProtoc(ctx, "latest")
	if err != nil {
		t.Fatalf("EnsureProtoc failed: %v", err)
	}

	if fi, err := os.Stat(protocBin); err != nil || fi.IsDir() {
		t.Fatalf("protoc binary not found at %s: %v", protocBin, err)
	}
	if fi, err := os.Stat(includeDir); err != nil || !fi.IsDir() {
		t.Fatalf("include dir not found at %s: %v", includeDir, err)
	}

	cmd := exec.CommandContext(ctx, protocBin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run protoc --version: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "libprotoc") && !strings.Contains(string(out), "protoc") {
		t.Errorf("unexpected output from protoc --version: %s", string(out))
	}
}

func TestFetchGitHubRelease_AuthorizationHeader(t *testing.T) {
	var gotAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"tag_name": "v36.2", "assets": []}`)); err != nil {
			return
		}
	}))
	defer server.Close()

	origClient := httpClient
	httpClient = server.Client()
	defer func() {
		httpClient = origClient
	}()

	t.Setenv("GITHUB_TOKEN", "test-bearer-token")
	_, err := fetchGitHubRelease(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("fetchGitHubRelease failed: %v", err)
	}

	wantAuth := "Bearer test-bearer-token"
	if gotAuthHeader != wantAuth {
		t.Errorf("got Authorization %q, want %q", gotAuthHeader, wantAuth)
	}
}

func TestEnsureProtoc_MemoizeLatest(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	// Pre-create the binary for version 36.2 so it doesn't need to download the asset
	binDir := filepath.Join(tempCache, "tools", "protoc", "36.2", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	binName := "protoc"
	if runtime.GOOS == "windows" {
		binName = "protoc.exe"
	}
	if err := os.WriteFile(filepath.Join(binDir, binName), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	assetName, err := ResolveProtocAsset("36.2", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		respJSON := fmt.Sprintf(`{
			"tag_name": "v36.2",
			"assets": [
				{"name": %q, "browser_download_url": "http://example.com/asset.zip", "digest": "sha256:1234"}
			]
		}`, assetName)
		if _, err := w.Write([]byte(respJSON)); err != nil {
			return
		}
	}))
	defer server.Close()

	origClient := httpClient
	httpClient = server.Client()
	defer func() {
		httpClient = origClient
	}()

	t.Setenv("PROTOC_DOWNLOAD_BASE_URL", server.URL)

	origNow := timeNow
	curTime := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time {
		return curTime
	}
	defer func() {
		timeNow = origNow
	}()

	// 1. First call: memo does not exist, so it should fetch release (requestCount = 1)
	bin, _, err := EnsureProtoc(t.Context(), "latest")
	if err != nil {
		t.Fatalf("EnsureProtoc failed: %v", err)
	}
	if !strings.Contains(bin, "36.2") {
		t.Errorf("expected 36.2 binary, got %s", bin)
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount)
	}

	memoPath := filepath.Join(tempCache, "tools", "protoc", "latest.json")
	if _, err := os.Stat(memoPath); err != nil {
		t.Fatalf("expected latest.json to exist at %s: %v", memoPath, err)
	}

	// 2. Second call 1 hour later: memo is within 24h, EnsureProtoc should use memo without network
	curTime = curTime.Add(1 * time.Hour)
	bin, _, err = EnsureProtoc(t.Context(), "latest")
	if err != nil {
		t.Fatalf("EnsureProtoc second call failed: %v", err)
	}
	if !strings.Contains(bin, "36.2") {
		t.Errorf("expected 36.2 binary, got %s", bin)
	}
	if requestCount != 1 {
		t.Errorf("expected still 1 request (memoized), got %d", requestCount)
	}

	// 3. Third call 25 hours later: memo has expired (> 24h), EnsureProtoc should re-fetch from network
	curTime = curTime.Add(25 * time.Hour)
	bin, _, err = EnsureProtoc(t.Context(), "latest")
	if err != nil {
		t.Fatalf("EnsureProtoc third call failed: %v", err)
	}
	if !strings.Contains(bin, "36.2") {
		t.Errorf("expected 36.2 binary, got %s", bin)
	}
	if requestCount != 2 {
		t.Errorf("expected 2 requests after memo expiration, got %d", requestCount)
	}
}
