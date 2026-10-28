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

package tasks

import (
	"context"
	"fmt"
	"strings"
)

// Task is the interface that all tasks must implement.
type Task interface {
	Run(ctx context.Context, scope *APScope) error
	GetName() string
	GetChildren() []Task
}

// FuncTask represents a task that executes a function.
type FuncTask struct {
	Name  string
	RunFn func(ctx context.Context, scope *APScope) error
}

func (t *FuncTask) Run(ctx context.Context, scope *APScope) error {
	return t.RunFn(ctx, scope)
}

func (t *FuncTask) GetName() string {
	return t.Name
}

func (t *FuncTask) GetChildren() []Task {
	return nil
}

// PrintTree prints the task tree to stdout.
func PrintTree(t Task, indent int) {
	fmt.Printf("%s%s\n", strings.Repeat("  ", indent), t.GetName())
	for _, child := range t.GetChildren() {
		PrintTree(child, indent+1)
	}
}
