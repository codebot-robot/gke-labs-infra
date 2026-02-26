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

package e2e

import (
	"os"
	"testing"
)

func TestNewHarness(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("Skipping E2E test; set RUN_E2E=1 to run")
	}

	h := NewHarness(t, Options{
		ClusterName: "ap-e2e-test",
	})

	if h.Namespace() == "" {
		t.Fatal("Namespace should not be empty")
	}

	h.KubectlApplyContent(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value
`)

	h.runCmd("kubectl", "get", "configmap", "test-cm", "-n", h.Namespace())
}
