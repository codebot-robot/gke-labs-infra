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

package e2e

import (
	"errors"
	"runtime"
	"testing"
)

func TestEnsureInotifyLimits_ReadBelowThreshold_WritesCalled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify limits are Linux-only")
	}

	origRead, origWrite := readSysctl, writeSysctl
	defer func() {
		readSysctl, writeSysctl = origRead, origWrite
	}()

	values := map[string]int{
		"fs.inotify.max_user_instances": 128,
		"fs.inotify.max_user_watches":   8192,
	}
	written := map[string]int{}

	readSysctl = func(key string) (int, error) {
		return values[key], nil
	}
	writeSysctl = func(key string, val int) error {
		written[key] = val
		values[key] = val
		return nil
	}

	EnsureInotifyLimits()

	if written["fs.inotify.max_user_instances"] != MinInotifyUserInstances {
		t.Errorf("expected max_user_instances written to %d, got %d", MinInotifyUserInstances, written["fs.inotify.max_user_instances"])
	}
	if written["fs.inotify.max_user_watches"] != MinInotifyUserWatches {
		t.Errorf("expected max_user_watches written to %d, got %d", MinInotifyUserWatches, written["fs.inotify.max_user_watches"])
	}
}

func TestEnsureInotifyLimits_ReadAtThreshold_NoWrite(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify limits are Linux-only")
	}

	origRead, origWrite := readSysctl, writeSysctl
	defer func() {
		readSysctl, writeSysctl = origRead, origWrite
	}()

	values := map[string]int{
		"fs.inotify.max_user_instances": 1024,
		"fs.inotify.max_user_watches":   1048576,
	}
	writeCalled := false

	readSysctl = func(key string) (int, error) {
		return values[key], nil
	}
	writeSysctl = func(key string, val int) error {
		writeCalled = true
		return nil
	}

	EnsureInotifyLimits()

	if writeCalled {
		t.Errorf("writeSysctl should not be called when limits are already at or above threshold")
	}
}

func TestEnsureInotifyLimits_WriteFails_WarnsAndContinues(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inotify limits are Linux-only")
	}

	origRead, origWrite := readSysctl, writeSysctl
	defer func() {
		readSysctl, writeSysctl = origRead, origWrite
	}()

	values := map[string]int{
		"fs.inotify.max_user_instances": 128,
		"fs.inotify.max_user_watches":   8192,
	}

	readSysctl = func(key string) (int, error) {
		return values[key], nil
	}
	writeSysctl = func(key string, val int) error {
		return errors.New("permission denied")
	}

	// Should log warning and not panic or fail
	EnsureInotifyLimits()
}

func TestEnsureInotifyLimits_RealEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		// Silent no-op on non-Linux
		EnsureInotifyLimits()
		return
	}

	instances, err := readSysctlInt("fs.inotify.max_user_instances")
	if err != nil {
		t.Fatalf("failed to read real fs.inotify.max_user_instances: %v", err)
	}
	if instances <= 0 {
		t.Errorf("expected positive fs.inotify.max_user_instances, got %d", instances)
	}

	watches, err := readSysctlInt("fs.inotify.max_user_watches")
	if err != nil {
		t.Fatalf("failed to read real fs.inotify.max_user_watches: %v", err)
	}
	if watches <= 0 {
		t.Errorf("expected positive fs.inotify.max_user_watches, got %d", watches)
	}

	// Calling EnsureInotifyLimits in real environment executes cleanly
	EnsureInotifyLimits()
}
