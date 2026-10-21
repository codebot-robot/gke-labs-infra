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
	"strings"
)

const (
	// DefaultProtocVersion is the default protoc version resolution.
	DefaultProtocVersion = "latest"
)

// ProtoConfig represents the protobuf configuration loaded from .ap/proto.yaml.
type ProtoConfig struct {
	Protoc  string            `json:"protoc"`
	Plugins map[string]string `json:"plugins"`
}

// DefaultProtoConfig returns a ProtoConfig with default protoc and plugin versions.
func DefaultProtoConfig() *ProtoConfig {
	return &ProtoConfig{
		Protoc:  DefaultProtocVersion,
		Plugins: make(map[string]string),
	}
}

// LoadProtoConfig loads and validates the protobuf configuration from .ap/proto.yaml in the specified directory.
// If the configuration file does not exist, it returns nil, nil.
func LoadProtoConfig(root string) (*ProtoConfig, error) {
	configFile := filepath.Join(root, ".ap/proto.yaml")

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading %s: %w", configFile, err)
	}

	var cfg ProtoConfig
	if err := UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
	}

	if err := cfg.ApplyDefaultsAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", configFile, err)
	}

	return &cfg, nil
}

// ApplyDefaultsAndValidate fills in defaults and validates the configuration fields.
func (c *ProtoConfig) ApplyDefaultsAndValidate() error {
	if c.Protoc == "" {
		c.Protoc = DefaultProtocVersion
	}

	if c.Plugins == nil {
		c.Plugins = make(map[string]string)
	} else {
		for name, ver := range c.Plugins {
			if ver == "" {
				return fmt.Errorf("plugin %q has empty version", name)
			}
			if ver != "latest" && !strings.HasPrefix(ver, "v") && !strings.Contains(ver, "/") {
				c.Plugins[name] = "v" + ver
			}
		}
	}

	return nil
}
