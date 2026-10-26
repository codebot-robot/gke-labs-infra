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
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ProtocEnvironment holds resolved binary paths and environment settings for invoking protoc.
type ProtocEnvironment struct {
	ProtocBin     string
	IncludeDir    string
	PluginBinDirs []string
	Env           []string
}

// DetectRequiredPlugins scans pass-through arguments for --<name>_out and --plugin=protoc-gen-<name>[=path].
// It returns a list of required plugin names (e.g. "protoc-gen-go").
func DetectRequiredPlugins(args []string) []string {
	var plugins []string
	seen := make(map[string]bool)

	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		flag := strings.TrimPrefix(arg, "--")

		// Case 1: --plugin=protoc-gen-<name>[=path]
		if strings.HasPrefix(flag, "plugin=") {
			val := strings.TrimPrefix(flag, "plugin=")
			// If format is name=path, user provided an explicit binary path
			if strings.Contains(val, "=") {
				continue
			}
			pluginName := val
			if !seen[pluginName] {
				seen[pluginName] = true
				plugins = append(plugins, pluginName)
			}
			continue
		}

		// Case 2: --<name>_out or --<name>_out=...
		var name string
		if strings.HasSuffix(flag, "_out") {
			name = strings.TrimSuffix(flag, "_out")
		} else if idx := strings.Index(flag, "_out="); idx != -1 {
			name = flag[:idx]
		}

		if name != "" {
			if protocBuiltinGenerators[name] {
				continue
			}
			pluginName := name
			if !strings.HasPrefix(pluginName, "protoc-gen-") {
				pluginName = "protoc-gen-" + pluginName
			}
			if !seen[pluginName] {
				seen[pluginName] = true
				plugins = append(plugins, pluginName)
			}
		}
	}

	sort.Strings(plugins)
	return plugins
}

// PrepareProtoc downloads/caches the required protoc and plugins based on config and arguments,
// and sets up an execution environment with plugins in PATH.
func PrepareProtoc(ctx context.Context, cfg *Config, args []string) (*ProtocEnvironment, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	protocBin, includeDir, err := EnsureProtoc(ctx, cfg.Protoc)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare protoc %s: %w", cfg.Protoc, err)
	}

	requiredPlugins := DetectRequiredPlugins(args)
	var pluginBinDirs []string

	for _, pluginName := range requiredPlugins {
		ver := cfg.Plugins[pluginName]
		if ver == "" {
			shortName := strings.TrimPrefix(pluginName, "protoc-gen-")
			ver = cfg.Plugins[shortName]
		}
		if ver == "" {
			ver = "latest"
		}

		binDir, err := EnsurePlugin(ctx, pluginName, ver, cfg.Plugins)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare plugin %s: %w", pluginName, err)
		}
		pluginBinDirs = append(pluginBinDirs, binDir)
	}

	var newPath string
	if len(pluginBinDirs) > 0 {
		newPath = strings.Join(pluginBinDirs, string(os.PathListSeparator)) + string(os.PathListSeparator) + os.Getenv("PATH")
	} else {
		newPath = os.Getenv("PATH")
	}

	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env = append(env, e)
		}
	}
	env = append(env, "PATH="+newPath)

	return &ProtocEnvironment{
		ProtocBin:     protocBin,
		IncludeDir:    includeDir,
		PluginBinDirs: pluginBinDirs,
		Env:           env,
	}, nil
}
