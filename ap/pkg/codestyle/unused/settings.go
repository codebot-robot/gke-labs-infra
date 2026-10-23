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
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"golang.org/x/tools/go/analysis"
)

// ParametersSettings declares the YAML configuration for unusedparameters in .ap/go.yaml.
type ParametersSettings struct {
	config.CommonLinterConfig
}

// CheckParameters reports whether unused parameter checking is enabled.
func (s *ParametersSettings) CheckParameters() bool {
	return s != nil && s.IsEnabled() && s.Mode != ""
}

// NewParametersSettings returns a new empty ParametersSettings instance.
func NewParametersSettings() config.LinterConfig {
	return &ParametersSettings{}
}

// NewSettings returns a new empty CommonLinterConfig instance for unused.
func NewSettings() config.LinterConfig {
	return &config.CommonLinterConfig{}
}

// New constructs the unused Analyzer from the resolved policy and loaded config.
func New(policy fileset.Policy, cfg *config.Config) *analysis.Analyzer {
	checkParams := false
	if cfg != nil {
		if ps, ok := cfg.Linter("unusedparameters").(*ParametersSettings); ok && ps != nil {
			checkParams = ps.CheckParameters()
		}
	}
	return NewAnalyzer(Config{
		Policy:          policy,
		CheckParameters: checkParams,
	})
}
