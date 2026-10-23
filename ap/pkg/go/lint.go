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

package golang

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
	"k8s.io/klog/v2"
)

var analyzerMu sync.Mutex

// GoVetTask represents a task to run go vet.
type GoVetTask struct {
	Dir string
}

func (t *GoVetTask) Run(ctx context.Context, scope *tasks.APScope) error {
	klog.Infof("Running go vet in %s", t.Dir)
	vetCmd := exec.CommandContext(ctx, "go", "vet", "./...")
	vetCmd.Dir = t.Dir
	vetCmd.Stdout = os.Stdout
	vetCmd.Stderr = os.Stderr
	if err := vetCmd.Run(); err != nil {
		return fmt.Errorf("go vet failed in %s: %w", t.Dir, err)
	}
	return nil
}

func (t *GoVetTask) GetName() string {
	return "go-vet"
}

func (t *GoVetTask) GetChildren() []tasks.Task {
	return nil
}

// GovulncheckTask represents a task to run govulncheck.
type GovulncheckTask struct {
	Dir string
}

func (t *GovulncheckTask) Run(ctx context.Context, scope *tasks.APScope) error {
	klog.Infof("Running govulncheck in %s", t.Dir)
	vulnCmd := exec.CommandContext(ctx, "go", "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...")
	vulnCmd.Dir = t.Dir
	vulnCmd.Stdout = os.Stdout
	vulnCmd.Stderr = os.Stderr
	if err := vulnCmd.Run(); err != nil {
		return fmt.Errorf("govulncheck failed in %s: %w", t.Dir, err)
	}
	return nil
}

func (t *GovulncheckTask) GetName() string {
	return "govulncheck"
}

func (t *GovulncheckTask) GetChildren() []tasks.Task {
	return nil
}

// AnalyzersTask represents a task to run enabled code style analyzers in-process.
type AnalyzersTask struct {
	Dir              string
	RepoRoot         string
	Enabled          map[string]codestyle.Mode
	PerAnalyzerFlags map[string][]string
	SkipGlobs        []string
}

// SkipGenerated reports whether skipGenerated is enabled for the named analyzer.
func (t *AnalyzersTask) SkipGenerated(name string) bool {
	for _, f := range t.PerAnalyzerFlags[name] {
		if strings.HasPrefix(f, "-skip-generated=") {
			return strings.TrimPrefix(f, "-skip-generated=") == "true"
		}
	}
	return false
}

// SkipTests reports whether skipTests is enabled for the named analyzer.
func (t *AnalyzersTask) SkipTests(name string) bool {
	for _, f := range t.PerAnalyzerFlags[name] {
		if strings.HasPrefix(f, "-skip-tests=") {
			return strings.TrimPrefix(f, "-skip-tests=") == "true"
		}
	}
	return false
}

// Packages returns any -package flags configured for the named analyzer.
func (t *AnalyzersTask) Packages(name string) []string {
	var pkgs []string
	for _, f := range t.PerAnalyzerFlags[name] {
		if strings.HasPrefix(f, "-package=") {
			pkgs = append(pkgs, strings.TrimPrefix(f, "-package="))
		}
	}
	return pkgs
}

// Types returns any -type flags configured for the named analyzer.
func (t *AnalyzersTask) Types(name string) []string {
	var types []string
	for _, f := range t.PerAnalyzerFlags[name] {
		if strings.HasPrefix(f, "-type=") {
			types = append(types, strings.TrimPrefix(f, "-type="))
		}
	}
	return types
}

func setAnalyzerFlag(fs *flag.FlagSet, analyzerName, flagStr string) error {
	flagStr = strings.TrimLeft(flagStr, "-")
	if idx := strings.Index(flagStr, "."); idx != -1 {
		prefix := flagStr[:idx]
		if strings.EqualFold(prefix, analyzerName) || strings.EqualFold(strings.ReplaceAll(prefix, "-", ""), strings.ReplaceAll(analyzerName, "-", "")) {
			flagStr = flagStr[idx+1:]
		}
	}
	k, v, hasVal := strings.Cut(flagStr, "=")
	if !hasVal {
		v = "true"
	}
	f := fs.Lookup(k)
	if f == nil {
		return fmt.Errorf("unknown flag %q for analyzer %s", k, analyzerName)
	}
	return fs.Set(k, v)
}

func (t *AnalyzersTask) Run(ctx context.Context, scope *tasks.APScope) error {
	klog.Infof("Running analyzers in %s", t.Dir)

	var analyzersToRun []*analysis.Analyzer
	for _, reg := range codestyle.Analyzers {
		if reg.CustomTask {
			continue
		}
		mode, ok := t.Enabled[reg.Name]
		if !ok || mode == codestyle.ModeIgnore {
			continue
		}
		analyzersToRun = append(analyzersToRun, reg.Analyzer)
	}
	if len(analyzersToRun) == 0 {
		return nil
	}

	analyzerMu.Lock()
	defer analyzerMu.Unlock()

	for _, a := range analyzersToRun {
		a.Flags.VisitAll(func(f *flag.Flag) {
			if r, ok := f.Value.(fileset.Resettable); ok {
				r.Reset()
			}
		})

		if t.RepoRoot != "" {
			if f := a.Flags.Lookup("repo-root"); f != nil {
				if err := a.Flags.Set("repo-root", t.RepoRoot); err != nil {
					return fmt.Errorf("failed to set repo-root for %s: %w", a.Name, err)
				}
			}
		}
		for _, g := range t.SkipGlobs {
			if f := a.Flags.Lookup("skip-glob"); f != nil {
				if err := a.Flags.Set("skip-glob", g); err != nil {
					return fmt.Errorf("failed to set skip-glob for %s: %w", a.Name, err)
				}
			}
		}
		for _, flagStr := range t.PerAnalyzerFlags[a.Name] {
			if err := setAnalyzerFlag(&a.Flags, a.Name, flagStr); err != nil {
				return err
			}
		}
	}

	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.LoadAllSyntax | packages.NeedModule,
		Dir:     t.Dir,
		Tests:   true,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("failed to load packages in %s: %w", t.Dir, err)
	}
	if len(pkgs) == 0 {
		return nil
	}

	if n := packages.PrintErrors(pkgs); n > 0 {
		return fmt.Errorf("failed to load packages in %s: %d error(s)", t.Dir, n)
	}

	graph, err := checker.Analyze(analyzersToRun, pkgs, nil)
	if err != nil {
		return fmt.Errorf("analyzers failed in %s: %w", t.Dir, err)
	}

	type diagKey struct {
		pos     token.Position
		end     token.Position
		name    string
		message string
	}
	seen := make(map[diagKey]bool)

	type positionedDiag struct {
		diag analysis.Diagnostic
		fset *token.FileSet
	}
	diagsByAnalyzer := make(map[string][]positionedDiag)
	var actionErrors []error

	for _, act := range graph.Roots {
		if act.Err != nil {
			actionErrors = append(actionErrors, fmt.Errorf("%s: %w", act.Analyzer.Name, act.Err))
			continue
		}
		if !act.IsRoot {
			continue
		}
		for _, diag := range act.Diagnostics {
			posn := act.Package.Fset.Position(diag.Pos)
			end := act.Package.Fset.Position(diag.End)
			k := diagKey{pos: posn, end: end, name: act.Analyzer.Name, message: diag.Message}
			if seen[k] {
				continue
			}
			seen[k] = true
			diagsByAnalyzer[act.Analyzer.Name] = append(diagsByAnalyzer[act.Analyzer.Name], positionedDiag{
				diag: diag,
				fset: act.Package.Fset,
			})
		}
	}

	var errs []error
	for _, a := range analyzersToRun {
		diags := diagsByAnalyzer[a.Name]
		if len(diags) == 0 {
			continue
		}
		sort.Slice(diags, func(i, j int) bool {
			pi := diags[i].fset.Position(diags[i].diag.Pos)
			pj := diags[j].fset.Position(diags[j].diag.Pos)
			if pi.Filename != pj.Filename {
				return pi.Filename < pj.Filename
			}
			if pi.Line != pj.Line {
				return pi.Line < pj.Line
			}
			return pi.Column < pj.Column
		})

		for _, pd := range diags {
			posn := pd.fset.Position(pd.diag.Pos)
			fmt.Fprintf(os.Stderr, "%s: %s\n", posn, pd.diag.Message)
			for _, rel := range pd.diag.Related {
				relPosn := pd.fset.Position(rel.Pos)
				fmt.Fprintf(os.Stderr, "%s:\t%s\n", relPosn, rel.Message)
			}
		}

		mode := t.Enabled[a.Name]
		if mode == codestyle.ModeError {
			errs = append(errs, fmt.Errorf("%s check failed in %s", a.Name, t.Dir))
		} else if mode == codestyle.ModeWarn {
			klog.Warningf("%s check failed in %s: %d diagnostics found", a.Name, t.Dir, len(diags))
		}
	}

	errs = append(errs, actionErrors...)
	return errors.Join(errs...)
}

func (t *AnalyzersTask) GetName() string {
	return "analyzers"
}

func (t *AnalyzersTask) GetChildren() []tasks.Task {
	return nil
}

// DroppedErrorsCheckTask represents a task to run droppederrors check.
type DroppedErrorsCheckTask struct {
	Dir           string
	RepoRoot      string
	IsError       bool
	SkipGenerated bool
	SkipTests     bool
	SkipGlobs     []string
}

func (t *DroppedErrorsCheckTask) Run(ctx context.Context, scope *tasks.APScope) error {
	klog.Infof("Running droppederrors check in %s", t.Dir)
	apPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find ap executable: %w", err)
	}
	args := []string{"lint", "droppederrors"}
	args = append(args, fmt.Sprintf("--skip-generated=%t", t.SkipGenerated))
	args = append(args, fmt.Sprintf("--skip-tests=%t", t.SkipTests))
	args = append(args, "./...")
	droppedCmd := exec.CommandContext(ctx, apPath, args...)
	droppedCmd.Dir = t.Dir
	droppedCmd.Stdout = os.Stdout
	droppedCmd.Stderr = os.Stderr
	if err := droppedCmd.Run(); err != nil {
		if t.IsError {
			return fmt.Errorf("droppederrors check failed in %s: %w", t.Dir, err)
		}
		klog.Warningf("droppederrors check failed in %s: %v", t.Dir, err)
	}
	return nil
}

func (t *DroppedErrorsCheckTask) GetName() string {
	return "droppederrors-check"
}

func (t *DroppedErrorsCheckTask) GetChildren() []tasks.Task {
	return nil
}

// LintTasks returns a task group for running go linting in discovered modules.
func LintTasks(r *repo.Repo, root string) (tasks.Task, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	modules := r.Under(root).Modules()

	var moduleTasks []tasks.Task
	for _, dir := range modules {
		hasGo, err := hasGoFiles(r, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to check for Go files in %s: %w", dir, err)
		}
		if !hasGo {
			continue
		}

		modGroup := &tasks.Group{
			Name:            fmt.Sprintf("go-lint-%s", filepath.Base(dir)),
			ContinueOnError: true,
		}

		if cfg.IsGovetEnabled() {
			modGroup.Tasks = append(modGroup.Tasks, &GoVetTask{Dir: dir})
		}
		if cfg.IsGovulncheckEnabled() {
			modGroup.Tasks = append(modGroup.Tasks, &GovulncheckTask{Dir: dir})
		}

		analyzersTask := &AnalyzersTask{
			Dir:              dir,
			RepoRoot:         root,
			Enabled:          make(map[string]codestyle.Mode),
			PerAnalyzerFlags: make(map[string][]string),
			SkipGlobs:        cfg.Skip,
		}

		for _, reg := range codestyle.Analyzers {
			if reg.CustomTask {
				if reg.Matches("droppederrors") {
					mode := reg.ResolveMode(cfg)
					if mode != codestyle.ModeIgnore {
						modGroup.Tasks = append(modGroup.Tasks, &DroppedErrorsCheckTask{
							Dir:           dir,
							RepoRoot:      root,
							IsError:       mode == codestyle.ModeError,
							SkipGenerated: reg.ResolveSkipGenerated(cfg),
							SkipTests:     reg.ResolveSkipTests(cfg),
							SkipGlobs:     cfg.Skip,
						})
					}
				}
				continue
			}

			mode := reg.ResolveMode(cfg)
			if mode == codestyle.ModeIgnore {
				continue
			}
			analyzersTask.Enabled[reg.Name] = mode
			var flags []string
			skipGen := reg.ResolveSkipGenerated(cfg)
			flags = append(flags, fmt.Sprintf("-skip-generated=%t", skipGen))
			skipTests := reg.ResolveSkipTests(cfg)
			flags = append(flags, fmt.Sprintf("-skip-tests=%t", skipTests))

			if reg.ExtraFlags != nil {
				flags = append(flags, reg.ExtraFlags(cfg)...)
			}
			analyzersTask.PerAnalyzerFlags[reg.Name] = flags
		}

		if len(analyzersTask.Enabled) > 0 {
			modGroup.Tasks = append(modGroup.Tasks, analyzersTask)
		}

		if len(modGroup.Tasks) > 0 {
			moduleTasks = append(moduleTasks, modGroup)
		}
	}

	return &tasks.Group{
		Name:            "go-lints",
		Tasks:           moduleTasks,
		ContinueOnError: true,
	}, nil
}

// Lint runs go vet and govulncheck in discovered modules.
func Lint(ctx context.Context, r *repo.Repo, root string) error {
	t, err := LintTasks(r, root)
	if err != nil {
		return err
	}
	return t.Run(ctx, &tasks.APScope{RepoRoot: root, Dir: root, Repo: r})
}

// hasGoFiles returns true if the directory or any of its subdirectories
// (excluding those that are themselves Go modules) contain at least one .go file.
func hasGoFiles(r *repo.Repo, root string) (bool, error) {
	return r.Under(root).HasGoFiles(), nil
}
