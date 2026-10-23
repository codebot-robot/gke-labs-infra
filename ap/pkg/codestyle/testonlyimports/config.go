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
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
)

// Config holds configuration for the testonlyimports analyzer.
type Config struct {
	config.CommonLinterConfig
	Packages []string `json:"packages"`
}

func init() {
	config.RegisterLinter("testOnlyImports", func() config.LinterConfig { return &Config{} })
}

// ExtraFlags returns the analyzer flags for the testonlyimports check based on its config.
func ExtraFlags(lc config.LinterConfig) []string {
	if c, ok := lc.(*Config); ok && c != nil {
		var flags []string
		for _, pkg := range c.Packages {
			flags = append(flags, "-package="+pkg)
		}
		return flags
	}
	return nil
}
