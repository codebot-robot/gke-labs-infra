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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"k8s.io/klog/v2"
)

const (
	// MinInotifyUserWatches is the recommended minimum value for fs.inotify.max_user_watches from kind.
	MinInotifyUserWatches = 524288

	// MinInotifyUserInstances is the recommended minimum value for fs.inotify.max_user_instances from kind.
	MinInotifyUserInstances = 512
)

var (
	readSysctl  = readSysctlInt
	writeSysctl = writeSysctlInt
)

// EnsureInotifyLimits checks fs.inotify.max_user_instances and fs.inotify.max_user_watches on Linux
// and raises them to at least kind's recommended minimums when possible.
func EnsureInotifyLimits() {
	if runtime.GOOS != "linux" {
		return
	}
	checkAndRaiseLimit("fs.inotify.max_user_instances", MinInotifyUserInstances)
	checkAndRaiseLimit("fs.inotify.max_user_watches", MinInotifyUserWatches)
}

func checkAndRaiseLimit(key string, minVal int) {
	currentVal, err := readSysctl(key)
	if err != nil {
		klog.Warningf("unable to read inotify limit %s: %v", key, err)
		return
	}
	if currentVal >= minVal {
		return
	}
	if err := writeSysctl(key, minVal); err != nil {
		klog.Warningf("unable to raise %s from %d to %d (recommended by kind): %v; kubelet may crash-loop if running multiple clusters or nodes", key, currentVal, minVal, err)
		return
	}
	newVal, err := readSysctl(key)
	if err != nil {
		klog.Infof("Raised %s from %d to %d", key, currentVal, minVal)
		return
	}
	klog.Infof("Raised %s from %d to %d", key, currentVal, newVal)
}

func readSysctlInt(key string) (int, error) {
	procPath := filepath.Join("/proc/sys", strings.ReplaceAll(key, ".", "/"))
	data, err := os.ReadFile(procPath)
	if err == nil {
		val, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if convErr != nil {
			return 0, fmt.Errorf("parsing %s: %w", procPath, convErr)
		}
		return val, nil
	}

	sysctlBin := findExecutable("sysctl")
	if sysctlBin == "" {
		return 0, fmt.Errorf("reading %s: %w (sysctl executable not found)", procPath, err)
	}

	cmd := exec.Command(sysctlBin, "-n", key)
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		return 0, fmt.Errorf("running %s -n %s: %w (output: %s)", sysctlBin, key, cmdErr, strings.TrimSpace(string(out)))
	}
	val, parseErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if parseErr != nil {
		return 0, fmt.Errorf("parsing %s output %q: %w", key, strings.TrimSpace(string(out)), parseErr)
	}
	return val, nil
}

func writeSysctlInt(key string, val int) error {
	sysctlBin := findExecutable("sysctl")
	if sysctlBin == "" {
		return fmt.Errorf("sysctl binary not found")
	}

	arg := fmt.Sprintf("%s=%d", key, val)

	if os.Geteuid() == 0 {
		cmd := exec.Command(sysctlBin, "-w", arg)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s -w %s failed: %w (output: %s)", sysctlBin, arg, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	sudoBin := findExecutable("sudo")
	var sudoErr error
	if sudoBin != "" {
		cmd := exec.Command(sudoBin, "-n", sysctlBin, "-w", arg)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		sudoErr = fmt.Errorf("sudo -n %s -w %s failed: %w (output: %s)", sysctlBin, arg, err, strings.TrimSpace(string(out)))
	}

	cmd := exec.Command(sysctlBin, "-w", arg)
	if out, err := cmd.CombinedOutput(); err != nil {
		if sudoErr != nil {
			return fmt.Errorf("%w; direct %s -w %s also failed: %v (output: %s)", sudoErr, sysctlBin, arg, err, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("%s -w %s failed: %w (output: %s)", sysctlBin, arg, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func findExecutable(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		for _, dir := range []string{"/usr/sbin", "/sbin", "/usr/bin", "/bin"} {
			c := filepath.Join(dir, name)
			if fileutils.IsExecutable(c) {
				return c
			}
		}
		return ""
	}
	return p
}
