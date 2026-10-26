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

package images

import (
	"os"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/imageref"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/k8s"
)

// ImageRef represents a reference to an image with prefix, name, and tag.
type ImageRef = imageref.ImageRef

// ImageNaming holds the resolved naming configuration for images.
type ImageNaming = imageref.ImageNaming

// ResolveImageNaming reads IMAGE_PREFIX and IMAGE_TAG environment variables exactly once
// and constructs an ImageNaming value.
func ResolveImageNaming(cfg *config.Config) ImageNaming {
	prefix := ""
	if cfg != nil {
		prefix = cfg.ImageRepo()
	} else {
		prefix = os.Getenv("IMAGE_PREFIX")
		if prefix == "" {
			prefix = "images.local"
		}
	}
	tag := os.Getenv("IMAGE_TAG")
	if tag == "" {
		tag = "latest"
	}
	return ImageNaming{
		Prefix:    prefix,
		Tag:       tag,
		InCluster: k8s.IsInCluster(),
	}
}
