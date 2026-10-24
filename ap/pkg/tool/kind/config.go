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
	"fmt"
	"os"
	"path/filepath"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
)

// Config represents the kind configuration loaded from .ap/kind.yaml.
type Config struct {
	Kind string `json:"kind"`
}

// DefaultConfig returns a Config with default kind version from the table.
func DefaultConfig() *Config {
	var kindVer string
	if rel := Latest(); rel != nil {
		kindVer = rel.Version
	}
	return &Config{
		Kind: kindVer,
	}
}

// LoadConfig loads and validates the kind configuration from .ap/kind.yaml in the specified directory.
// If the configuration file does not exist, it returns nil, nil.
func LoadConfig(root string) (*Config, error) {
	configFile := filepath.Join(root, ".ap/kind.yaml")

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading %s: %w", configFile, err)
	}

	var cfg Config
	if err := config.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
	}

	if err := cfg.ApplyDefaultsAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", configFile, err)
	}

	return &cfg, nil
}

// ApplyDefaultsAndValidate fills in defaults and validates configuration fields.
func (c *Config) ApplyDefaultsAndValidate() error {
	if c.Kind == "" || c.Kind == "latest" {
		if rel := Latest(); rel != nil {
			c.Kind = rel.Version
		}
	}
	return nil
}
