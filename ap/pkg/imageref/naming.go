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

package imageref

import "strings"

// ImageRef represents a reference to an image with prefix, name, and tag.
// Tag is never empty ("latest" when unset).
type ImageRef struct {
	Prefix string
	Name   string
	Tag    string
}

// String returns the image reference formatted as prefix/name:tag, or name:tag if prefix is empty.
func (r ImageRef) String() string {
	tag := r.Tag
	if tag == "" {
		tag = "latest"
	}
	if r.Prefix != "" {
		return r.Prefix + "/" + r.Name + ":" + tag
	}
	return r.Name + ":" + tag
}

// ImageNaming holds the naming conventions for images in a build or deploy run.
type ImageNaming struct {
	Prefix    string
	Tag       string
	InCluster bool
}

// Ref returns an ImageRef for the given image name.
func (n ImageNaming) Ref(name string) ImageRef {
	tag := n.Tag
	if tag == "" {
		tag = "latest"
	}
	return ImageRef{
		Prefix: n.Prefix,
		Name:   name,
		Tag:    tag,
	}
}

// PushPrefix returns the registry prefix to use when pushing images.
// For the default "images.local" prefix, it rewrites to the in-cluster registry
// (if running in-cluster or using a k8s buildkit host) or localhost:5000.
// For any other prefix, it returns Prefix unchanged.
func (n ImageNaming) PushPrefix(buildkitHost string) string {
	if n.Prefix == "images.local" {
		if n.InCluster || strings.HasPrefix(buildkitHost, "k8s://") {
			return "in-cluster-image-registry.in-cluster-image-registry-system.svc.cluster.local:80"
		}
		return "localhost:5000"
	}
	return n.Prefix
}
