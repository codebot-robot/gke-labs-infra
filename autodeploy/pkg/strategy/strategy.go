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

package strategy

// Strategy defines when a commit should be deployed.
type Strategy interface {
	ShouldDeploy(commitHash string, branch string, tags []string) bool
}

// AlwaysDeploy strategy deploys every commit on the tracked branch.
type AlwaysDeploy struct{}

func (s *AlwaysDeploy) ShouldDeploy(commitHash string, branch string, tags []string) bool {
	return true
}

// TagDeploy strategy deploys only when a new tag is found.
type TagDeploy struct {
	TagPattern string
}

func (s *TagDeploy) ShouldDeploy(commitHash string, branch string, tags []string) bool {
	// TODO: Implement tag pattern matching
	return len(tags) > 0
}
