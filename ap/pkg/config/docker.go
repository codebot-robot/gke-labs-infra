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

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/docker"
)

// DockerConfig represents the docker and buildx configuration loaded from .ap/docker.yaml.
type DockerConfig struct {
	Docker string `json:"docker"`
	Buildx string `json:"buildx"`
}

// DefaultDockerConfig returns a DockerConfig with default docker and buildx versions from the table.
func DefaultDockerConfig() *DockerConfig {
	var dockerVer, buildxVer string
	if rel := docker.LatestDocker(); rel != nil {
		dockerVer = rel.Version
	}
	if rel := docker.LatestBuildx(); rel != nil {
		buildxVer = rel.Version
	}
	return &DockerConfig{
		Docker: dockerVer,
		Buildx: buildxVer,
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
	if c.Docker == "" || c.Docker == "latest" {
		if rel := docker.LatestDocker(); rel != nil {
			c.Docker = rel.Version
		}
	}
	if c.Buildx == "" || c.Buildx == "latest" {
		if rel := docker.LatestBuildx(); rel != nil {
			c.Buildx = rel.Version
		}
	}
	return nil
}
