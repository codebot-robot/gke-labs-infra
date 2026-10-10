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

package rules

import (
	"fmt"

	"github.com/gke-labs/gke-labs-infra/kubelint/pkg/manifests"
	"github.com/gke-labs/gke-labs-infra/kubelint/rules"
)

type StatefulSetUpdateStrategy struct {
	name    string
	message string
}

func (r *StatefulSetUpdateStrategy) init() {
	if r.name == "" {
		r.name, r.message = ParseRuleMarkdown(ruledata.StatefulSetUpdateStrategyMD)
	}
}

func (r *StatefulSetUpdateStrategy) Name() string {
	r.init()
	return r.name
}

func getLine(obj *manifests.Object, path string, fallbacks ...string) int {
	line, err := obj.GetLine(path)
	if err != nil {
		for _, fb := range fallbacks {
			fbLine, fbErr := obj.GetLine(fb)
			if fbErr != nil {
				continue
			}
			return fbLine
		}
		if obj.Node != nil && obj.Node.Line > 0 {
			return obj.Node.Line
		}
		return 1
	}
	return line
}

func (r *StatefulSetUpdateStrategy) Check(obj *manifests.Object) []Diagnostic {
	r.init()
	kind, _, err := obj.Kind()
	if err != nil {
		return []Diagnostic{
			{
				RuleName: r.Name(),
				Message:  fmt.Sprintf("malformed manifest: failed to read kind: %v", err),
				Line:     getLine(obj, "kind"),
			},
		}
	}
	if kind != "StatefulSet" {
		return nil
	}

	_, found, err := obj.GetString("spec.updateStrategy.type")
	if err != nil {
		return []Diagnostic{
			{
				RuleName: r.Name(),
				Message:  fmt.Sprintf("malformed manifest: failed to read spec.updateStrategy.type: %v", err),
				Line:     getLine(obj, "spec.updateStrategy.type", "spec.updateStrategy", "kind"),
			},
		}
	}
	if !found {
		// Also check if spec.updateStrategy is set but type is missing (though type is required if updateStrategy is present)
		_, stratFound, stratErr := obj.GetString("spec.updateStrategy")
		if stratErr != nil {
			return []Diagnostic{
				{
					RuleName: r.Name(),
					Message:  r.message,
					Line:     getLine(obj, "spec.updateStrategy", "kind"),
				},
			}
		}
		if !stratFound {
			return []Diagnostic{
				{
					RuleName: r.Name(),
					Message:  r.message,
					Line:     getLine(obj, "kind"),
				},
			}
		}
	}

	return nil
}
