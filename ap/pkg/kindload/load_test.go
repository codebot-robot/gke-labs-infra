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

package kindload

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

func TestLoadImages_RequiresKindBin(t *testing.T) {
	err := LoadImages(t.Context(), "", "ap-cluster", []string{"test-image:latest"})
	if err == nil {
		t.Fatal("expected error when kindBin is empty, got nil")
	}
	if !strings.Contains(err.Error(), "kindBin is required") {
		t.Fatalf("expected error to contain 'kindBin is required', got: %v", err)
	}

	err = LoadImages(t.Context(), "", "ap-cluster", nil)
	if err == nil {
		t.Fatal("expected error when kindBin is empty and images is nil, got nil")
	}
}

func TestLoadImages_EmptyImages(t *testing.T) {
	err := LoadImages(t.Context(), "kind", "ap-cluster", nil)
	if err != nil {
		t.Fatalf("expected nil when images is empty, got: %v", err)
	}
	err = LoadImages(t.Context(), "kind", "ap-cluster", []string{})
	if err != nil {
		t.Fatalf("expected nil when images is empty, got: %v", err)
	}
}

func TestLoadImages_Retry(t *testing.T) {
	origBackoffs := DefaultBackoffs
	DefaultBackoffs = []time.Duration{
		time.Millisecond,
		time.Millisecond,
		time.Millisecond,
		time.Millisecond,
		time.Millisecond,
	}
	t.Cleanup(func() {
		DefaultBackoffs = origBackoffs
	})

	t.Run("transient failure followed by success loads", func(t *testing.T) {
		tempDir := t.TempDir()
		countFile := filepath.Join(tempDir, "count")
		fakeBin := filepath.Join(tempDir, "kind")

		script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
echo "$count" > %q
if [ "$count" -le 1 ]; then
  echo "transient containerd socket error" >&2
  exit 1
fi
exit 0
`, countFile, countFile)

		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}

		var logBuf bytes.Buffer
		klog.SetOutput(&logBuf)
		klog.LogToStderr(false)
		defer func() {
			klog.SetOutput(os.Stderr)
			klog.LogToStderr(true)
		}()

		err := LoadImages(t.Context(), fakeBin, "test-cluster", []string{"test-image:latest"})
		if err != nil {
			t.Fatalf("expected LoadImages to succeed on retry, got: %v", err)
		}

		rawCount, err := os.ReadFile(countFile)
		if err != nil {
			t.Fatalf("failed to read count file: %v", err)
		}
		count, err := strconv.Atoi(strings.TrimSpace(string(rawCount)))
		if err != nil {
			t.Fatalf("failed to parse count: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", count)
		}

		logOutput := logBuf.String()
		if !strings.Contains(logOutput, "failed to load image(s) into kind cluster \"test-cluster\" (attempt 1/6)") {
			t.Errorf("expected log to contain retry warning for attempt 1/6, got: %s", logOutput)
		}
	})

	t.Run("persistent failure returns last error after attempts", func(t *testing.T) {
		tempDir := t.TempDir()
		countFile := filepath.Join(tempDir, "count")
		fakeBin := filepath.Join(tempDir, "kind")

		script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
echo "$count" > %q
echo "persistent containerd failure" >&2
exit 1
`, countFile, countFile)

		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}

		var logBuf bytes.Buffer
		klog.SetOutput(&logBuf)
		klog.LogToStderr(false)
		defer func() {
			klog.SetOutput(os.Stderr)
			klog.LogToStderr(true)
		}()

		err := LoadImages(t.Context(), fakeBin, "test-cluster", []string{"test-image:latest"})
		if err == nil {
			t.Fatal("expected LoadImages to fail on persistent error, got nil")
		}

		if !strings.Contains(err.Error(), "failed to load image(s) into kind cluster \"test-cluster\"") {
			t.Errorf("expected error message to contain cluster name, got: %v", err)
		}
		if !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("expected error message to wrap last exit error, got: %v", err)
		}

		rawCount, err := os.ReadFile(countFile)
		if err != nil {
			t.Fatalf("failed to read count file: %v", err)
		}
		count, err := strconv.Atoi(strings.TrimSpace(string(rawCount)))
		if err != nil {
			t.Fatalf("failed to parse count: %v", err)
		}
		expectedAttempts := len(DefaultBackoffs) + 1
		if count != expectedAttempts {
			t.Errorf("expected %d attempts, got %d", expectedAttempts, count)
		}

		logOutput := logBuf.String()
		for attempt := 1; attempt <= len(DefaultBackoffs); attempt++ {
			expectedWarning := fmt.Sprintf("failed to load image(s) into kind cluster \"test-cluster\" (attempt %d/%d)", attempt, expectedAttempts)
			if !strings.Contains(logOutput, expectedWarning) {
				t.Errorf("expected log to contain retry warning %q, got: %s", expectedWarning, logOutput)
			}
		}
	})

	t.Run("context cancellation aborts retry", func(t *testing.T) {
		tempDir := t.TempDir()
		countFile := filepath.Join(tempDir, "count")
		fakeBin := filepath.Join(tempDir, "kind")

		script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
echo "$count" > %q
exit 1
`, countFile, countFile)

		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}

		var logBuf bytes.Buffer
		klog.SetOutput(&logBuf)
		klog.LogToStderr(false)
		defer func() {
			klog.SetOutput(os.Stderr)
			klog.LogToStderr(true)
		}()

		ctx, cancel := context.WithCancel(t.Context())

		orig := DefaultBackoffs
		DefaultBackoffs = []time.Duration{500 * time.Millisecond, 500 * time.Millisecond}
		defer func() { DefaultBackoffs = orig }()

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		err := LoadImages(ctx, fakeBin, "test-cluster", []string{"test-image:latest"})
		if err == nil {
			t.Fatal("expected error on cancelled context, got nil")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected error to contain 'context canceled', got: %v", err)
		}
	})
}
