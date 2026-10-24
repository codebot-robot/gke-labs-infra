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
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultDockerVersion is the default docker CLI version.
	DefaultDockerVersion = "28.0.1"
	// DefaultBuildxVersion is the default docker-buildx plugin version.
	DefaultBuildxVersion = "v0.21.1"
)

// DockerConfig represents the docker and docker-buildx configuration loaded from .ap/docker.yaml.
type DockerConfig struct {
	Docker       string `json:"docker"`
	Buildx       string `json:"buildx"`
	DockerBuildx string `json:"docker-buildx"`
}

// DefaultDockerConfig returns a DockerConfig with default docker and buildx versions.
func DefaultDockerConfig() *DockerConfig {
	return &DockerConfig{
		Docker: DefaultDockerVersion,
		Buildx: DefaultBuildxVersion,
	}
}

// LoadDockerConfig loads and validates the docker configuration from .ap/docker.yaml in the specified directory.
// If the configuration file does not exist, it returns nil, nil.
func LoadDockerConfig(root string) (*DockerConfig, error) {
	configFile := filepath.Join(root, ".ap/docker.yaml")

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading %s: %w", configFile, err)
	}

	var cfg DockerConfig
	if err := UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
	}

	if err := cfg.ApplyDefaultsAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", configFile, err)
	}

	return &cfg, nil
}

// ApplyDefaultsAndValidate fills in defaults and validates configuration fields.
func (c *DockerConfig) ApplyDefaultsAndValidate() error {
	if c.Buildx == "" && c.DockerBuildx != "" {
		c.Buildx = c.DockerBuildx
	}
	if c.Docker == "" {
		c.Docker = DefaultDockerVersion
	}
	if c.Buildx == "" {
		c.Buildx = DefaultBuildxVersion
	}
	return nil
}
