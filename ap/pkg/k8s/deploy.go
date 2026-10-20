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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	apimachineryyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

type imageReferenceMatch struct {
	start     int
	end       int
	imageName string
}

const (
	imagePrefixPlaceholder = "${IMAGE_PREFIX}/"
	imageTagPlaceholder    = ":${IMAGE_TAG}"
)

func isAlphaNumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isImageNameChar(b byte) bool {
	return isAlphaNumeric(b) || b == '.' || b == '_' || b == '-' || b == '/'
}

func findImageReferenceMatches(s string) []imageReferenceMatch {
	var matches []imageReferenceMatch
	offset := 0
	for {
		idx := strings.Index(s[offset:], imagePrefixPlaceholder)
		if idx == -1 {
			break
		}
		start := offset + idx
		nameStart := start + len(imagePrefixPlaceholder)

		if nameStart >= len(s) || !isAlphaNumeric(s[nameStart]) {
			offset = nameStart
			continue
		}

		nameEnd := nameStart + 1
		for nameEnd < len(s) && isImageNameChar(s[nameEnd]) {
			nameEnd++
		}

		if !isAlphaNumeric(s[nameEnd-1]) {
			offset = nameEnd
			continue
		}

		imageName := s[nameStart:nameEnd]
		if strings.Contains(imageName, "//") {
			offset = nameEnd
			continue
		}

		fullEnd := nameEnd
		if nameEnd < len(s) && s[nameEnd] == ':' {
			if strings.HasPrefix(s[nameEnd:], imageTagPlaceholder) {
				tagEnd := nameEnd + len(imageTagPlaceholder)
				if tagEnd < len(s) && (isAlphaNumeric(s[tagEnd]) || s[tagEnd] == '-' || s[tagEnd] == '_' || s[tagEnd] == '.') {
					offset = nameEnd
					continue
				}
				fullEnd = tagEnd
			} else {
				// Non-matching tag (e.g. :latest or :v1)
				offset = nameEnd
				continue
			}
		}

		matches = append(matches, imageReferenceMatch{
			start:     start,
			end:       fullEnd,
			imageName: imageName,
		})

		offset = fullEnd
	}
	return matches
}

func collectStringImageReferences(node *yaml.Node, seen map[string]bool, names *[]string) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectStringImageReferences(child, seen, names)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			valueNode := node.Content[i+1]
			if valueNode.Kind == yaml.ScalarNode {
				for _, match := range findImageReferenceMatches(valueNode.Value) {
					if !seen[match.imageName] {
						seen[match.imageName] = true
						*names = append(*names, match.imageName)
					}
				}
			}
			collectStringImageReferences(valueNode, seen, names)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if child.Kind == yaml.ScalarNode {
				for _, match := range findImageReferenceMatches(child.Value) {
					if !seen[match.imageName] {
						seen[match.imageName] = true
						*names = append(*names, match.imageName)
					}
				}
			}
			collectStringImageReferences(child, seen, names)
		}
	}
}

// FindPlaceholderImages parses YAML content and returns a deduplicated, sorted slice
// of placeholder image names referenced in the content.
func FindPlaceholderImages(content string) ([]string, error) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	var placeholders []*yaml.Node
	seen := make(map[string]bool)
	var names []string

	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}
		placeholders = collectPlaceholders(&node, placeholders, nil)
		collectStringImageReferences(&node, seen, &names)
	}

	for _, p := range placeholders {
		base, ok := isPlaceholderImage(p.Value)
		if !ok {
			continue
		}
		if !seen[base] {
			seen[base] = true
			names = append(names, base)
		}
	}

	if len(names) == 0 {
		return nil, nil
	}

	sort.Strings(names)
	return names, nil
}

// FindReferencedImagesInFile reads a YAML manifest file and returns referenced placeholder image names.
func FindReferencedImagesInFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return FindPlaceholderImages(string(content))
}

func replacePlaceholderImages(content string, imageRepository string, imageTag string) (string, error) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	var placeholders []*yaml.Node
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to decode YAML: %w", err)
		}
		placeholders = collectPlaceholders(&node, placeholders, nil)
	}

	lineOffsets := getLineOffsets(content)

	type replacement struct {
		offset int
		length int
		newVal string
	}
	var replacements []replacement

	for _, p := range placeholders {
		if p.Line == 0 || p.Line > len(lineOffsets) {
			return "", fmt.Errorf("invalid line number %d for placeholder %q", p.Line, p.Value)
		}
		start := lineOffsets[p.Line-1] + p.Column - 1
		if start >= len(content) {
			return "", fmt.Errorf("invalid column %d on line %d for placeholder %q", p.Column, p.Line, p.Value)
		}

		end := findEnd(content, start, p.Style)

		base, ok := isPlaceholderImage(p.Value)
		if !ok {
			return "", fmt.Errorf("invalid placeholder image %q", p.Value)
		}

		newVal := fmt.Sprintf("%s/%s:%s", imageRepository, base, imageTag)
		replacements = append(replacements, replacement{
			offset: start,
			length: end - start,
			newVal: newVal,
		})
	}

	for _, match := range findImageReferenceMatches(content) {
		newVal := fmt.Sprintf("%s/%s:%s", imageRepository, match.imageName, imageTag)
		replacements = append(replacements, replacement{
			offset: match.start,
			length: match.end - match.start,
			newVal: newVal,
		})
	}

	if len(replacements) == 0 {
		return content, nil
	}

	// Sort replacements in reverse order to apply them without affecting offsets
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].offset > replacements[j].offset
	})

	for _, r := range replacements {
		content = content[:r.offset] + r.newVal + content[r.offset+r.length:]
	}

	return content, nil
}

func collectPlaceholders(node *yaml.Node, placeholders []*yaml.Node, path []string) []*yaml.Node {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			placeholders = collectPlaceholders(child, placeholders, path)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			newPath := append(path, keyNode.Value)
			if keyNode.Value == "image" && valueNode.Kind == yaml.ScalarNode {
				if isImageField(newPath) {
					if _, ok := isPlaceholderImage(valueNode.Value); ok {
						placeholders = append(placeholders, valueNode)
					}
				}
			}
			placeholders = collectPlaceholders(valueNode, placeholders, newPath)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			placeholders = collectPlaceholders(child, placeholders, append(path, "*"))
		}
	}
	return placeholders
}

func isPlaceholderImage(image string) (string, bool) {
	if image == "" {
		return "", false
	}

	if strings.HasPrefix(image, "${") {
		return "", false
	}

	// Handle digest if any (we probably want to skip these too)
	if strings.Contains(image, "@") {
		return "", false
	}

	base := image
	tag := ""
	if i := strings.LastIndex(image, ":"); i != -1 {
		lastPart := image[i+1:]
		if !strings.Contains(lastPart, "/") {
			base = image[:i]
			tag = lastPart
		}
	}

	if tag != "" && tag != "latest" {
		return "", false
	}

	// Check for host
	firstSlash := strings.Index(image, "/")
	if firstSlash != -1 {
		host := image[:firstSlash]
		if strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost" {
			return "", false
		}
	}

	return base, true
}

func isImageField(path []string) bool {
	p := strings.Join(path, ".")
	switch p {
	case "image",
		"spec.containers.*.image",
		"spec.initContainers.*.image",
		"spec.template.spec.containers.*.image",
		"spec.template.spec.initContainers.*.image",
		"spec.jobTemplate.spec.template.spec.containers.*.image",
		"spec.jobTemplate.spec.template.spec.initContainers.*.image",
		"spec.podTemplate.spec.containers.*.image",
		"spec.podTemplate.spec.initContainers.*.image":
		return true
	}
	return false
}

func getLineOffsets(content string) []int {
	offsets := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func findEnd(content string, start int, style yaml.Style) int {
	if style&yaml.DoubleQuotedStyle != 0 {
		for i := start + 1; i < len(content); i++ {
			if content[i] == '"' {
				backslashes := 0
				for j := i - 1; j >= start; j-- {
					if content[j] == '\\' {
						backslashes++
					} else {
						break
					}
				}
				if backslashes%2 == 0 {
					return i + 1
				}
			}
		}
	} else if style&yaml.SingleQuotedStyle != 0 {
		for i := start + 1; i < len(content); i++ {
			if content[i] == '\'' {
				if i+1 < len(content) && content[i+1] == '\'' {
					i++
					continue
				}
				return i + 1
			}
		}
	} else {
		for i := start; i < len(content); i++ {
			c := content[i]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#' || c == ',' || c == ']' || c == '}' {
				return i
			}
		}
	}
	return len(content)
}

// KubectlApplyTask represents a task to apply a single k8s manifest using client-go Server-Side Apply.
type KubectlApplyTask struct {
	ManifestPath string

	// DynamicClient is an optional client-go dynamic client (defaults to cluster dynamic client).
	DynamicClient dynamic.Interface

	// RESTMapper is an optional RESTMapper (defaults to cluster discovery RESTMapper).
	RESTMapper meta.RESTMapper

	// FieldManager is the field manager used for Server-Side Apply (defaults to "ap").
	FieldManager string
}

func (t *KubectlApplyTask) Run(ctx context.Context, scope *tasks.APScope) error {
	cfg, err := config.Load(scope.RepoRoot)
	if err != nil {
		return err
	}
	imageRepository := cfg.ImageRepo()
	if imageRepository == "" {
		return fmt.Errorf("IMAGE_PREFIX is not set; it is required for deploy")
	}
	tag := os.Getenv("IMAGE_TAG")
	if tag == "" {
		tag = "latest"
	}

	relPath, err := filepath.Rel(scope.RepoRoot, t.ManifestPath)
	if err != nil {
		return err
	}
	klog.Infof("Applying manifest %s", relPath)

	content, err := os.ReadFile(t.ManifestPath)
	if err != nil {
		return err
	}

	replaced, err := replacePlaceholderImages(string(content), imageRepository, tag)
	if err != nil {
		return fmt.Errorf("failed to replace placeholders in %s: %w", relPath, err)
	}

	dynClient := t.DynamicClient
	mapper := t.RESTMapper
	if dynClient == nil || mapper == nil {
		defaultDyn, defaultMapper, err := GetDefaultClientAndMapper()
		if err != nil {
			return fmt.Errorf("failed to initialize kubernetes client for %s: %w", relPath, err)
		}
		if dynClient == nil {
			dynClient = defaultDyn
		}
		if mapper == nil {
			mapper = defaultMapper
		}
	}

	fieldManager := t.FieldManager
	if fieldManager == "" {
		fieldManager = "ap"
	}

	if err := ApplyManifest(ctx, dynClient, mapper, replaced, fieldManager); err != nil {
		return fmt.Errorf("apply failed for %s: %w", relPath, err)
	}

	return nil
}

func (t *KubectlApplyTask) GetName() string {
	return fmt.Sprintf("kubectl-apply-%s", filepath.Base(t.ManifestPath))
}

func (t *KubectlApplyTask) GetChildren() []tasks.Task {
	return nil
}

// ApplyManifest applies multi-document Kubernetes YAML content using client-go Server-Side Apply (SSA).
func ApplyManifest(ctx context.Context, dynamicClient dynamic.Interface, restMapper meta.RESTMapper, manifestContent string, fieldManager string) error {
	if fieldManager == "" {
		fieldManager = "ap"
	}

	decoder := apimachineryyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestContent), 4096)
	for {
		var rawObj unstructured.Unstructured
		err := decoder.Decode(&rawObj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to decode YAML object: %w", err)
		}
		if len(rawObj.Object) == 0 {
			continue
		}

		gvk := rawObj.GroupVersionKind()
		mapping, err := restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			if resetter, ok := restMapper.(interface{ Reset() }); ok {
				resetter.Reset()
				mapping, err = restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			}
		}
		if err != nil {
			return fmt.Errorf("failed to find REST mapping for %s: %w", gvk, err)
		}

		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			ns := rawObj.GetNamespace()
			if ns == "" {
				ns = "default"
			}
			dr = dynamicClient.Resource(mapping.Resource).Namespace(ns)
		} else {
			dr = dynamicClient.Resource(mapping.Resource)
		}

		data, err := json.Marshal(rawObj.Object)
		if err != nil {
			return fmt.Errorf("failed to marshal object %s/%s: %w", gvk.Kind, rawObj.GetName(), err)
		}

		force := true
		patchOpts := metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        &force,
		}

		klog.V(2).Infof("Server-Side Apply %s/%s (GVR: %s)", gvk.Kind, rawObj.GetName(), mapping.Resource)
		_, err = dr.Patch(ctx, rawObj.GetName(), types.ApplyPatchType, data, patchOpts)
		if err != nil {
			return fmt.Errorf("failed to apply %s/%s: %w", gvk.Kind, rawObj.GetName(), err)
		}

		// If this is a CRD, wait for it to become established
		if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
			if err := WaitForCRDEstablished(ctx, dynamicClient, rawObj.GetName(), 60*time.Second); err != nil {
				return fmt.Errorf("failed waiting for CRD %s: %w", rawObj.GetName(), err)
			}
			if resetter, ok := restMapper.(interface{ Reset() }); ok {
				resetter.Reset()
			}
		}
	}

	return nil
}

// WaitForCRDEstablished waits until the CRD with the given name has condition Established=True.
func WaitForCRDEstablished(ctx context.Context, dynamicClient dynamic.Interface, crdName string, timeout time.Duration) error {
	klog.Infof("Waiting for CRD %s to be established...", crdName)
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	pollInterval := 500 * time.Millisecond
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj, err := dynamicClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if err != nil || !found {
			return false, nil
		}

		for _, c := range conditions {
			condMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)
			if condType == "Established" && condStatus == "True" {
				return true, nil
			}
		}

		return false, nil
	})
}

// DeployTasksForManifests returns a task group for deploying the specified k8s manifests.
func DeployTasksForManifests(manifests []string) tasks.Task {
	var deployTasks []tasks.Task
	for _, manifest := range manifests {
		deployTasks = append(deployTasks, &KubectlApplyTask{
			ManifestPath: manifest,
		})
	}

	return &tasks.Group{
		Name:  "deploy-k8s",
		Tasks: deployTasks,
	}
}

// DeployTasks returns a task group for deploying all k8s manifests found in k8s directories.
func DeployTasks(r *repo.Repo, root string) (tasks.Task, error) {
	manifests, err := findManifests(r, root)
	if err != nil {
		return nil, err
	}
	return DeployTasksForManifests(manifests), nil
}

// Deploy deploys k8s manifests found in k8s directories.
func Deploy(ctx context.Context, r *repo.Repo, root string) error {
	t, err := DeployTasks(r, root)
	if err != nil {
		return err
	}
	return t.Run(ctx, &tasks.APScope{RepoRoot: root, Dir: root, Repo: r})
}

func findManifests(r *repo.Repo, root string) ([]string, error) {
	return r.Under(root).FindManifests(), nil
}
