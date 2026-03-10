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

package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/apis/infra/v1alpha1"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/executor"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/git"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/strategy"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AutoDeployReconciler reconciles an AutoDeploy object
type AutoDeployReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Runner executor.Runner
}

// Reconcile checks for updates and triggers deployments if necessary.
func (r *AutoDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.Infof("Reconciling AutoDeploy %s", req.NamespacedName)

	var ad v1alpha1.AutoDeploy
	if err := r.Get(ctx, req.NamespacedName, &ad); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	repoURL := ad.Spec.Repo
	branch := ad.Spec.Branch
	if branch == "" {
		branch = "main"
	}

	pollInterval := 1 * time.Minute
	if ad.Spec.Interval != "" {
		if d, err := time.ParseDuration(ad.Spec.Interval); err == nil {
			pollInterval = d
		}
	}

	monitor := git.NewMonitor(repoURL)
	strat := &strategy.AlwaysDeploy{}
	runner := r.Runner
	if runner == nil {
		runner = &executor.APRunner{
			ImagePrefix: os.Getenv("IMAGE_PREFIX"),
			DockerHost:  os.Getenv("DOCKER_HOST"),
		}
	}

	commit, err := monitor.GetLatestCommit(ctx, branch)
	if err != nil {
		return ctrl.Result{RequeueAfter: pollInterval}, fmt.Errorf("failed to get latest commit: %w", err)
	}

	if commit == "" {
		klog.V(4).Info("No commits found yet")
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}

	if ad.Status.LastDeployedCommit == commit {
		klog.V(4).Infof("Commit %s already deployed", commit)
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}

	if strat.ShouldDeploy(commit, branch, nil) {
		klog.Infof("Triggering deployment for commit %s", commit)

		tempDir, err := os.MkdirTemp("", "autodeploy-*")
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if err := monitor.Clone(ctx, branch, tempDir); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clone repo: %w", err)
		}

		var args []string
		if ad.Spec.Directory != "" {
			args = append(args, "--root="+ad.Spec.Directory)
		}

		if err := runner.DeployFlow(ctx, tempDir, args...); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to run deploy flow: %w", err)
		}

		// For now just update status to simulate success
		ad.Status.LastDeployedCommit = commit
		if err := r.Status().Update(ctx, &ad); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: pollInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutoDeployReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AutoDeploy{}).
		Complete(r)
}
