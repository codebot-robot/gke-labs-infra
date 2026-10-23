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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

type Config struct {
	Gofmt       *GofmtConfig       `json:"gofmt"`
	Govet       *GovetConfig       `json:"govet"`
	Govulncheck *GovulncheckConfig `json:"govulncheck"`
	Skip        []string           `json:"skip"`
	Lint        *LintConfig        `json:"lint"`
}

type GofmtConfig struct {
	Enabled *bool `json:"enabled"`
}

type GovetConfig struct {
	Enabled *bool `json:"enabled"`
}

type GovulncheckConfig struct {
	Enabled *bool `json:"enabled"`
}

type LintConfig struct {
	SkipGenerated *bool `json:"skipGenerated"`
	SkipTests     *bool `json:"skipTests"`

	linters map[string]LinterConfig
}

// UnmarshalJSON unmarshals shared lint config and dynamic per-linter configurations.
func (l *LintConfig) UnmarshalJSON(data []byte) error {
	type plain LintConfig
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	l.SkipGenerated = p.SkipGenerated
	l.SkipTests = p.SkipTests

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	linterMu.RLock()
	defer linterMu.RUnlock()

	if l.linters == nil {
		l.linters = make(map[string]LinterConfig)
	}

	for key, rawVal := range raw {
		if key == "skipGenerated" || key == "skipTests" {
			continue
		}
		newFn, ok := linterRegistry[key]
		if !ok {
			continue
		}
		cfg := newFn()
		if len(rawVal) > 0 && string(rawVal) != "null" {
			if err := json.Unmarshal(rawVal, cfg); err != nil {
				return fmt.Errorf("error unmarshaling %s: %w", key, err)
			}
		}
		l.linters[key] = cfg
	}
	return nil
}

// LinterConfig represents common configuration options for a linter.
type LinterConfig interface {
	IsEnabled() bool
	IsError() bool
	GetMode() string
	SkipGeneratedOverride() *bool
	SkipTestsOverride() *bool
}

// CommonLinterConfig holds configuration common across linters.
type CommonLinterConfig struct {
	Mode          string `json:"mode"`
	SkipGenerated *bool  `json:"skipGenerated"`
	SkipTests     *bool  `json:"skipTests"`
}

var _ LinterConfig = (*CommonLinterConfig)(nil)

type UnusedConfig struct {
	CommonLinterConfig
}

type ReplaceEmptyInterfaceWithAnyConfig struct {
	CommonLinterConfig
}

type TestContextConfig struct {
	CommonLinterConfig
}

type DeferInLoopConfig struct {
	CommonLinterConfig
}

type UnusedParametersConfig struct {
	CommonLinterConfig
}

type DroppedErrorsConfig struct {
	CommonLinterConfig
	Exclude            []string `json:"exclude"`
	Baseline           string   `json:"baseline"`
	UseDefaultExcludes *bool    `json:"useDefaultExcludes"`
	GOOS               []string `json:"goos"`
}

type TestOnlyImportsConfig struct {
	CommonLinterConfig
	Packages []string `json:"packages"`
}

type CloseWriterConfig struct {
	CommonLinterConfig
	Types []string `json:"types"`
}

// Linter returns the LinterConfig for the named linter.
// It panics if the linter name is not registered.
// If the linter is not explicitly configured, it returns a nil-safe LinterConfig with defaults.
func (c *Config) Linter(name string) LinterConfig {
	linterMu.RLock()
	_, ok := linterRegistry[name]
	canon, isAlias := aliasToCanon[name]
	aliases := canonToAliases[name]
	linterMu.RUnlock()
	if !ok {
		panic(fmt.Sprintf("unknown linter %q", name))
	}
	if c == nil || c.Lint == nil || c.Lint.linters == nil {
		return (*CommonLinterConfig)(nil)
	}
	if lc, ok := c.Lint.linters[name]; ok && lc != nil {
		return lc
	}
	if isAlias {
		if lc, ok := c.Lint.linters[canon]; ok && lc != nil {
			return lc
		}
	}
	for _, alias := range aliases {
		if lc, ok := c.Lint.linters[alias]; ok && lc != nil {
			return lc
		}
	}
	return (*CommonLinterConfig)(nil)
}

// Load loads the configuration from .ap/go.yaml in the repository root.
func Load(repoRoot string) (*Config, error) {
	configFile := filepath.Join(repoRoot, ".ap/go.yaml")

	var config Config
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", configFile, err)
		}

		if err := UnmarshalStrict(data, &config); err != nil {
			return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking %s: %w", configFile, err)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// Validate checks the configuration for semantic errors.
func (c *Config) Validate() error {
	if c.Lint != nil {
		checkMode := func(name, m string) error {
			if m != "" && m != "error" && m != "warn" && m != "ignore" {
				return fmt.Errorf("invalid %s mode %q: must be one of 'error', 'warn', 'ignore'", name, m)
			}
			return nil
		}
		names := RegisteredLinters()
		for _, name := range names {
			lc := c.Linter(name)
			if lc != nil {
				if err := checkMode(name, lc.GetMode()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// IsGofmtEnabled returns true if gofmt is enabled in the config (defaulting to true).
func (c *Config) IsGofmtEnabled() bool {
	if c.Gofmt != nil && c.Gofmt.Enabled != nil {
		return *c.Gofmt.Enabled
	}
	return true
}

// IsGovetEnabled returns true if govet is enabled in the config (defaulting to true).
func (c *Config) IsGovetEnabled() bool {
	if c.Govet != nil && c.Govet.Enabled != nil {
		return *c.Govet.Enabled
	}
	return true
}

// IsGovulncheckEnabled returns true if govulncheck is enabled in the config (defaulting to true).
func (c *Config) IsGovulncheckEnabled() bool {
	if c.Govulncheck != nil && c.Govulncheck.Enabled != nil {
		return *c.Govulncheck.Enabled
	}
	return true
}

// IsUnusedEnabled returns true if unused detection is enabled in the config (defaulting to true, mode != "ignore").
func (c *Config) IsUnusedEnabled() bool {
	return c.Linter("unused").IsEnabled()
}

// IsUnusedParametersEnabled returns true if unused parameter detection is enabled.
// Default is false.
func (c *Config) IsUnusedParametersEnabled() bool {
	if c == nil || c.Lint == nil {
		return false
	}
	lc := c.Linter("unused")
	if lc != nil {
		val := reflect.ValueOf(lc)
		method := val.MethodByName("CheckParameters")
		if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			res := method.Call(nil)
			if b, ok := res[0].Interface().(bool); ok && b {
				return true
			}
		}
	}
	linterMu.RLock()
	_, hasUnusedParams := linterRegistry["unusedparameters"]
	linterMu.RUnlock()
	if hasUnusedParams {
		up := c.Linter("unusedparameters")
		if up != nil && up.IsEnabled() && up.GetMode() != "" {
			return true
		}
	}
	return false
}

// IsTestContextEnabled returns true if testcontext detection is enabled in the config (defaulting to true, mode != "ignore").
func (c *Config) IsTestContextEnabled() bool {
	return c.Linter("testcontext").IsEnabled()
}

// IsTestContextError returns true if testcontext should be reported as an error.
// Default is false (warning).
func (c *Config) IsTestContextError() bool {
	return c.Linter("testcontext").GetMode() == "error"
}

// IsReplaceEmptyInterfaceWithAnyEnabled returns true if the replace-empty-interface-with-any linter is enabled in the config (defaulting to true, mode != "ignore").
func (c *Config) IsReplaceEmptyInterfaceWithAnyEnabled() bool {
	return c.Linter("replaceEmptyInterfaceWithAny").IsEnabled()
}

// IsDroppedErrorsEnabled returns true if dropped error checking is enabled (defaulting to true, mode != "ignore").
func (c *Config) IsDroppedErrorsEnabled() bool {
	return c.Linter("droppedErrors").IsEnabled()
}

// IsDroppedErrorsError returns true if dropped errors should be reported as an error.
// Default is true ("error" mode).
func (c *Config) IsDroppedErrorsError() bool {
	return c.DroppedErrorsMode() == "error"
}

// DroppedErrorsMode returns the mode for dropped error checking: "ignore", "warn", or "error".
// Default is "error".
func (c *Config) DroppedErrorsMode() string {
	m := c.Linter("droppedErrors").GetMode()
	if m != "" {
		return m
	}
	return "error"
}

// DroppedErrors returns the configuration for droppedErrors, or nil if not configured.
func (c *Config) DroppedErrors() *DroppedErrorsConfig {
	lc := c.Linter("droppedErrors")
	if de, ok := lc.(*DroppedErrorsConfig); ok {
		return de
	}
	return nil
}

// DroppedErrorsBaseline returns the configured baseline file path, or empty string.
func (c *Config) DroppedErrorsBaseline() string {
	if de := c.DroppedErrors(); de != nil {
		return de.Baseline
	}
	return ""
}

// DroppedErrorsExclude returns the configured excluded symbols.
func (c *Config) DroppedErrorsExclude() []string {
	if de := c.DroppedErrors(); de != nil {
		return de.Exclude
	}
	return nil
}

// GetMode returns the configured mode, or empty string if unset.
func (c *CommonLinterConfig) GetMode() string {
	if c == nil {
		return ""
	}
	return c.Mode
}

// IsEnabled returns true if the linter is enabled (defaulting to true, mode != "ignore").
func (c *CommonLinterConfig) IsEnabled() bool {
	if c != nil && c.Mode != "" {
		return c.Mode != "ignore"
	}
	return true
}

// IsError returns true if violations should be reported as errors (defaulting to true, mode == "error").
func (c *CommonLinterConfig) IsError() bool {
	if c != nil && c.Mode != "" {
		return c.Mode == "error"
	}
	return true
}

// SkipGeneratedOverride returns the configured skipGenerated override, or nil if unset.
func (c *CommonLinterConfig) SkipGeneratedOverride() *bool {
	if c == nil {
		return nil
	}
	return c.SkipGenerated
}

// SkipTestsOverride returns the configured skipTests override, or nil if unset.
func (c *CommonLinterConfig) SkipTestsOverride() *bool {
	if c == nil {
		return nil
	}
	return c.SkipTests
}

// TestOnlyImports returns the configuration for testOnlyImports, or nil if not configured.
func (c *Config) TestOnlyImports() *TestOnlyImportsConfig {
	if c == nil || c.Lint == nil {
		return nil
	}
	lc := c.Linter("testOnlyImports")
	if lc == nil {
		return nil
	}
	if toi, ok := lc.(*TestOnlyImportsConfig); ok {
		return toi
	}
	val := reflect.ValueOf(lc)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		f := val.FieldByName("Packages")
		if f.IsValid() && f.Type() == reflect.TypeOf([]string(nil)) {
			pkgs := f.Interface().([]string)
			return &TestOnlyImportsConfig{
				CommonLinterConfig: CommonLinterConfig{
					Mode:          lc.GetMode(),
					SkipGenerated: lc.SkipGeneratedOverride(),
					SkipTests:     lc.SkipTestsOverride(),
				},
				Packages: pkgs,
			}
		}
	}
	return nil
}

// CloseWriter returns the configuration for closeWriter, or nil if not configured.
func (c *Config) CloseWriter() *CloseWriterConfig {
	if c == nil || c.Lint == nil {
		return nil
	}
	lc := c.Linter("closeWriter")
	if lc == nil {
		return nil
	}
	if cw, ok := lc.(*CloseWriterConfig); ok {
		return cw
	}
	val := reflect.ValueOf(lc)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		f := val.FieldByName("Types")
		if f.IsValid() && f.Type() == reflect.TypeOf([]string(nil)) {
			types := f.Interface().([]string)
			return &CloseWriterConfig{
				CommonLinterConfig: CommonLinterConfig{
					Mode:          lc.GetMode(),
					SkipGenerated: lc.SkipGeneratedOverride(),
					SkipTests:     lc.SkipTestsOverride(),
				},
				Types: types,
			}
		}
	}
	return nil
}

// DeferInLoop returns the configuration for deferInLoop, or nil if not configured.
func (c *Config) DeferInLoop() *DeferInLoopConfig {
	if c != nil && c.Lint != nil {
		lc := c.Linter("deferInLoop")
		if lc != nil && lc.GetMode() != "" {
			return &DeferInLoopConfig{
				CommonLinterConfig: CommonLinterConfig{
					Mode:          lc.GetMode(),
					SkipGenerated: lc.SkipGeneratedOverride(),
					SkipTests:     lc.SkipTestsOverride(),
				},
			}
		}
	}
	return nil
}

// IsLintSkipGenerated returns true if generated files should be skipped across lint checks (default false).
func (c *Config) IsLintSkipGenerated() bool {
	if c.Lint != nil && c.Lint.SkipGenerated != nil {
		return *c.Lint.SkipGenerated
	}
	return false
}

// IsLintSkipTests returns true if test files should be skipped across lint checks (default false).
func (c *Config) IsLintSkipTests() bool {
	if c.Lint != nil && c.Lint.SkipTests != nil {
		return *c.Lint.SkipTests
	}
	return false
}

// ResolveSkipGenerated resolves whether generated files should be skipped,
// checking a per-check override first, then lint.skipGenerated, then analyzerDefault.
func (c *Config) ResolveSkipGenerated(lc LinterConfig, analyzerDefault bool) bool {
	var override *bool
	if lc != nil {
		override = lc.SkipGeneratedOverride()
	}
	var shared *bool
	if c != nil && c.Lint != nil {
		shared = c.Lint.SkipGenerated
	}
	return resolve(override, shared, analyzerDefault)
}

// ResolveSkipTests resolves whether test files should be skipped,
// checking a per-check override first, then lint.skipTests, then analyzerDefault.
func (c *Config) ResolveSkipTests(lc LinterConfig, analyzerDefault bool) bool {
	var override *bool
	if lc != nil {
		override = lc.SkipTestsOverride()
	}
	var shared *bool
	if c != nil && c.Lint != nil {
		shared = c.Lint.SkipTests
	}
	return resolve(override, shared, analyzerDefault)
}

func resolve(override *bool, shared *bool, analyzerDefault bool) bool {
	if override != nil {
		return *override
	}
	if shared != nil {
		return *shared
	}
	return analyzerDefault
}

// DroppedErrorsSkipTests returns true if test files should be skipped (default true).
func (c *Config) DroppedErrorsSkipTests() bool {
	return c.ResolveSkipTests(c.Linter("droppedErrors"), true)
}

// DroppedErrorsSkipGenerated returns true if generated files should be skipped (default false).
func (c *Config) DroppedErrorsSkipGenerated() bool {
	return c.ResolveSkipGenerated(c.Linter("droppedErrors"), false)
}

// DroppedErrorsUseDefaultExcludes returns true if default exclusions should be used (default true).
func (c *Config) DroppedErrorsUseDefaultExcludes() bool {
	if de := c.DroppedErrors(); de != nil && de.UseDefaultExcludes != nil {
		return *de.UseDefaultExcludes
	}
	return true
}

// DroppedErrorsGOOS returns the list of GOOS targets to check.
// Default is host plus "linux" (or just "linux" if host is linux).
func (c *Config) DroppedErrorsGOOS() []string {
	if de := c.DroppedErrors(); de != nil && len(de.GOOS) > 0 {
		return de.GOOS
	}
	host := runtime.GOOS
	if host == "linux" {
		return []string{"linux"}
	}
	return []string{host, "linux"}
}

// ImageRepo returns the image repository to use, defaulting to "images.local".
func (c *Config) ImageRepo() string {
	repo := os.Getenv("IMAGE_PREFIX")
	if repo == "" {
		return "images.local"
	}
	return repo
}

type HeadersConfig struct {
	License         string   `json:"license"`
	CopyrightHolder string   `json:"copyrightHolder"`
	Skip            []string `json:"skip"`
	SkipGenerated   *bool    `json:"skipGenerated"`
}

func LoadHeaders(repoRoot string) (*HeadersConfig, error) {
	configFile := filepath.Join(repoRoot, ".ap/headers.yaml")
	var config HeadersConfig
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", configFile, err)
		}

		if err := UnmarshalStrict(data, &config); err != nil {
			return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking %s: %w", configFile, err)
	}

	if config.SkipGenerated == nil {
		t := true
		config.SkipGenerated = &t
	}
	return &config, nil
}

// ImagesConfig represents the image build configuration, loaded from .ap/images.yaml.
type ImagesConfig struct {
	Platforms []string `json:"platforms"`
}

// LoadImagesConfig loads the configuration from .ap/images.yaml in the specified root directory.
func LoadImagesConfig(root string) (*ImagesConfig, error) {
	configFile := filepath.Join(root, ".ap/images.yaml")

	var config ImagesConfig
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", configFile, err)
		}

		if err := UnmarshalStrict(data, &config); err != nil {
			return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking %s: %w", configFile, err)
	}

	return &config, nil
}

// GetPlatforms returns the configured platforms, defaulting to ["linux/amd64", "linux/arm64"] if none are specified,
// and normalizing short names like "amd64" or "arm64" to "linux/amd64" or "linux/arm64".
func (c *ImagesConfig) GetPlatforms() []string {
	if len(c.Platforms) == 0 {
		return []string{"linux/amd64", "linux/arm64"}
	}
	var normalized []string
	for _, p := range c.Platforms {
		p = strings.TrimSpace(p)
		if p == "amd64" {
			normalized = append(normalized, "linux/amd64")
		} else if p == "arm64" {
			normalized = append(normalized, "linux/arm64")
		} else if p != "" {
			normalized = append(normalized, p)
		}
	}
	return normalized
}
