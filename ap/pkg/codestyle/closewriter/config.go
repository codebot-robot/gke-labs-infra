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

package closewriter

import (
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
)

// Config holds configuration for the closewriter analyzer.
type Config struct {
	config.CommonLinterConfig
	Types []string `json:"types"`
}

func init() {
	config.RegisterLinter("closeWriter", func() config.LinterConfig { return &Config{} })
}

// ExtraFlags returns the analyzer flags for the closewriter check based on its config.
func ExtraFlags(lc config.LinterConfig) []string {
	if c, ok := lc.(*Config); ok && c != nil {
		var flags []string
		for _, typ := range c.Types {
			flags = append(flags, "-type="+typ)
		}
		return flags
	}
	return nil
}
