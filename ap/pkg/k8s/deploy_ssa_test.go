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

package k8s

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func newTestRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "apiextensions.k8s.io", Version: "v1"},
	})
	mapper.AddSpecific(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"},
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespace"},
		meta.RESTScopeRoot,
	)
	mapper.AddSpecific(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmap"},
		meta.RESTScopeNamespace,
	)
	mapper.AddSpecific(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployment"},
		meta.RESTScopeNamespace,
	)
	mapper.AddSpecific(
		schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
		schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinition"},
		meta.RESTScopeRoot,
	)
	return mapper
}

func TestApplyManifest_ServerSideApply(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	fakeClient.PrependReactor("patch", "*", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		patchAction := action.(clienttesting.PatchAction)
		obj := &unstructured.Unstructured{}
		obj.SetName(patchAction.GetName())
		obj.SetNamespace(patchAction.GetNamespace())
		return true, obj, nil
	})

	mapper := newTestRESTMapper()

	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test-ns
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: test-ns
data:
  key: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: test-ns
spec:
  replicas: 1
`

	ctx := t.Context()
	if err := ApplyManifest(ctx, fakeClient, mapper, manifest, "ap"); err != nil {
		t.Fatalf("ApplyManifest failed: %v", err)
	}

	actions := fakeClient.Actions()
	if len(actions) != 3 {
		t.Fatalf("expected 3 patch actions, got %d", len(actions))
	}

	expected := []struct {
		resource  string
		name      string
		namespace string
	}{
		{"namespaces", "test-ns", ""},
		{"configmaps", "test-cm", "test-ns"},
		{"deployments", "test-deploy", "test-ns"},
	}

	for i, exp := range expected {
		action, ok := actions[i].(clienttesting.PatchActionImpl)
		if !ok {
			t.Fatalf("action %d is not a PatchActionImpl: %T", i, actions[i])
		}
		if action.GetResource().Resource != exp.resource {
			t.Errorf("action %d resource = %s, want %s", i, action.GetResource().Resource, exp.resource)
		}
		if action.GetName() != exp.name {
			t.Errorf("action %d name = %s, want %s", i, action.GetName(), exp.name)
		}
		if action.GetNamespace() != exp.namespace {
			t.Errorf("action %d namespace = %s, want %s", i, action.GetNamespace(), exp.namespace)
		}
		if action.GetPatchType() != types.ApplyPatchType {
			t.Errorf("action %d patchType = %s, want %s", i, action.GetPatchType(), types.ApplyPatchType)
		}
		opts := action.GetPatchOptions()
		if opts.FieldManager != "ap" {
			t.Errorf("action %d fieldManager = %s, want 'ap'", i, opts.FieldManager)
		}
		if opts.Force == nil || !*opts.Force {
			t.Errorf("action %d force = %v, want true", i, opts.Force)
		}
	}
}

func TestApplyManifest_CRDEstablished(t *testing.T) {
	scheme := runtime.NewScheme()
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	crdObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": "widgets.example.com",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Established",
						"status": "True",
					},
				},
			},
		},
	}

	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, crdObj)
	fakeClient.PrependReactor("patch", "customresourcedefinitions", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, crdObj, nil
	})

	mapper := newTestRESTMapper()

	manifest := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
`

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := ApplyManifest(ctx, fakeClient, mapper, manifest, "ap"); err != nil {
		t.Fatalf("ApplyManifest failed on CRD: %v", err)
	}

	actions := fakeClient.Actions()
	hasPatch := false
	hasGet := false
	for _, a := range actions {
		if a.GetVerb() == "patch" && a.GetResource() == crdGVR {
			hasPatch = true
		}
		if a.GetVerb() == "get" && a.GetResource() == crdGVR {
			hasGet = true
		}
	}
	if !hasPatch {
		t.Errorf("expected patch action for CRD")
	}
	if !hasGet {
		t.Errorf("expected get action verifying CRD condition")
	}
}

func TestCheckNamespaceExists(t *testing.T) {
	scheme := runtime.NewScheme()
	existingNs := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": "active-ns",
			},
		},
	}

	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, existingNs)

	ctx := t.Context()
	if err := CheckNamespaceExists(ctx, fakeClient, "active-ns"); err != nil {
		t.Errorf("expected active-ns to exist, got error: %v", err)
	}

	if err := CheckNamespaceExists(ctx, fakeClient, "missing-ns"); err == nil {
		t.Errorf("expected missing-ns to return error, got nil")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error message to contain 'does not exist', got: %v", err)
	}
}

func TestKubectlApplyTask_Run_WithInjectedClient(t *testing.T) {
	t.Setenv("IMAGE_PREFIX", "gcr.io/test-project")
	tmpDir := t.TempDir()

	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}
	apYAML := `imageRepo: gcr.io/test-project
`
	if err := os.WriteFile(filepath.Join(apDir, "ap.yaml"), []byte(apYAML), 0644); err != nil {
		t.Fatal(err)
	}

	k8sDir := filepath.Join(tmpDir, "k8s")
	if err := os.MkdirAll(k8sDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(k8sDir, "app.yaml")
	manifestContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deployment
  namespace: test-ns
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-app:latest
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	var capturedPatch []byte
	fakeClient.PrependReactor("patch", "deployments", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		patchAction := action.(clienttesting.PatchAction)
		capturedPatch = patchAction.GetPatch()
		obj := &unstructured.Unstructured{}
		obj.SetName(patchAction.GetName())
		obj.SetNamespace(patchAction.GetNamespace())
		return true, obj, nil
	})

	mapper := newTestRESTMapper()

	task := &KubectlApplyTask{
		ManifestPath:  manifestPath,
		DynamicClient: fakeClient,
		RESTMapper:    mapper,
		FieldManager:  "ap",
	}

	scope := &tasks.APScope{
		RepoRoot: tmpDir,
		Dir:      tmpDir,
	}

	if err := task.Run(t.Context(), scope); err != nil {
		t.Fatalf("task.Run failed: %v", err)
	}

	if len(capturedPatch) == 0 {
		t.Fatalf("expected patch to be captured, got none")
	}

	patchStr := string(capturedPatch)
	if !strings.Contains(patchStr, "gcr.io/test-project/my-app:latest") {
		t.Errorf("expected placeholder replacement in patch data, got:\n%s", patchStr)
	}
}
