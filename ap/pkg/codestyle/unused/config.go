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

package unused

import (
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
)

// ParametersConfig holds configuration for unused parameter detection.
type ParametersConfig struct {
	config.CommonLinterConfig
}

// Config holds configuration for the unused analyzer.
type Config struct {
	config.CommonLinterConfig
	Parameters *ParametersConfig `json:"parameters"`
}

func init() {
	config.RegisterLinter("unused", func() config.LinterConfig { return &Config{} })
	config.RegisterLinter("unusedparameters", func() config.LinterConfig { return &ParametersConfig{} })
}

// ExtraFlags returns the analyzer flags for the unused check based on its config.
func ExtraFlags(lc config.LinterConfig) []string {
	if u, ok := lc.(*Config); ok && u != nil && u.Parameters != nil && u.Parameters.Mode != "skip" {
		return []string{"-check-parameters=true"}
	}
	return []string{"-check-parameters=false"}
}
