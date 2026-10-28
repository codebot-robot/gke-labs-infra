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
	"errors"
	"strings"

	"k8s.io/klog/v2"
)

// Group represents a collection of tasks.
type Group struct {
	Name            string
	Tasks           []Task
	ContinueOnError bool
}

func (g *Group) Run(ctx context.Context, scope *APScope) error {
	var errs []error
	var failedNames []string
	for _, t := range g.Tasks {
		if err := RunTask(ctx, scope, t); err != nil {
			if !g.ContinueOnError {
				return err
			}
			errs = append(errs, err)
			failedNames = append(failedNames, strings.TrimSuffix(t.GetName(), "-check"))
		}
	}
	if len(errs) > 0 {
		if len(g.Tasks) > 1 {
			klog.Errorf("%s: %d of %d failed: %s", g.Name, len(failedNames), len(g.Tasks), strings.Join(failedNames, ", "))
		}
		return errors.Join(errs...)
	}
	return nil
}

func (g *Group) GetName() string {
	return g.Name
}

func (g *Group) GetChildren() []Task {
	return g.Tasks
}

// RunOptions holds options for running tasks.
type RunOptions struct {
	DryRun          bool
	ContinueOnError bool
}

// Run executes a list of tasks.
func Run(ctx context.Context, scope *APScope, ts []Task, opts RunOptions) error {
	if opts.DryRun {
		for _, task := range ts {
			PrintTree(task, 0)
		}
		return nil
	}
	defer func() {
		if scope != nil && scope.Repo != nil {
			if saveErr := scope.Repo.Save(); saveErr != nil {
				klog.V(2).Infof("failed to save repo hash cache: %v", saveErr)
			}
		}
	}()
	var errs []error
	for _, task := range ts {
		if err := RunTask(ctx, scope, task); err != nil {
			if !opts.ContinueOnError {
				return err
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
