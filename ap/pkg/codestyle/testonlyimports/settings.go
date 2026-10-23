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

package testonlyimports

import (
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"golang.org/x/tools/go/analysis"
)

// Settings declares the YAML configuration for testOnlyImports in .ap/go.yaml.
type Settings struct {
	config.CommonLinterConfig
	Packages []string `json:"packages"`
}

// NewSettings returns a new empty Settings instance.
func NewSettings() config.LinterConfig {
	return &Settings{}
}

// New constructs the testonlyimports Analyzer from the resolved policy and linter settings.
func New(policy fileset.Policy, lc config.LinterConfig) *analysis.Analyzer {
	cfg := Config{Policy: policy}
	if s, ok := lc.(*Settings); ok && s != nil {
		cfg.Packages = s.Packages
	}
	return NewAnalyzer(cfg)
}
