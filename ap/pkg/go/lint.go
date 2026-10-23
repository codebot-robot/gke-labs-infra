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
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/closewriter"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/droppederrors"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/testonlyimports"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/walker"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
	"k8s.io/klog/v2"
)

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

type positionedDiag struct {
	diag analysis.Diagnostic
	fset *token.FileSet
}

// AnalyzersTask represents a task to run enabled code style analyzers in-process.
type AnalyzersTask struct {
	Dir           string
	RepoRoot      string
	Analyzers     []*analysis.Analyzer
	Enabled       map[string]codestyle.Mode
	Policies      map[string]fileset.Policy
	Settings      map[string]config.LinterConfig
	GOOS          []string
	HostGOOS      string
	LoadCount     int
	WriteBaseline bool
	BaselinePath  string
}

// SkipGenerated reports whether skipGenerated is enabled for the named analyzer.
func (t *AnalyzersTask) SkipGenerated(name string) bool {
	if p, ok := t.Policies[name]; ok {
		return p.SkipGenerated
	}
	return false
}

// SkipTests reports whether skipTests is enabled for the named analyzer.
func (t *AnalyzersTask) SkipTests(name string) bool {
	if p, ok := t.Policies[name]; ok {
		return p.SkipTests
	}
	return false
}

// Packages returns any packages configured for the named analyzer.
func (t *AnalyzersTask) Packages(name string) []string {
	if s, ok := t.Settings[name].(*testonlyimports.Settings); ok && s != nil {
		return s.Packages
	}
	return nil
}

// Types returns any types configured for the named analyzer.
func (t *AnalyzersTask) Types(name string) []string {
	if s, ok := t.Settings[name].(*closewriter.Settings); ok && s != nil {
		return s.Types
	}
	return nil
}

func (t *AnalyzersTask) Run(ctx context.Context, scope *tasks.APScope) error {
	klog.Infof("Running analyzers in %s", t.Dir)

	if len(t.Analyzers) == 0 {
		return nil
	}

	targetGOOSList := t.GOOS
	if len(targetGOOSList) == 0 {
		host := t.HostGOOS
		if host == "" {
			host = runtime.GOOS
		}
		targetGOOSList = []string{host}
	}

	type diagKey struct {
		pos     token.Position
		end     token.Position
		name    string
		message string
	}
	seen := make(map[diagKey]bool)
	seenFindings := make(map[string]bool)
	var allDroppedErrorsFindings []droppederrors.Finding

	diagsByAnalyzer := make(map[string][]positionedDiag)
	allCheckedFiles := make(map[string]bool)
	var actionErrors []error

	for _, targetGOOS := range targetGOOSList {
		cfg := &packages.Config{
			Context: ctx,
			Mode:    packages.LoadAllSyntax | packages.NeedModule,
			Dir:     t.Dir,
			Tests:   true,
		}
		if targetGOOS != runtime.GOOS {
			cfg.Env = append(os.Environ(), "GOOS="+targetGOOS)
		}

		t.LoadCount++
		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			return fmt.Errorf("failed to load packages in %s: %w", t.Dir, err)
		}
		if len(pkgs) == 0 {
			continue
		}

		if n := packages.PrintErrors(pkgs); n > 0 {
			return fmt.Errorf("failed to load packages in %s: %d error(s)", t.Dir, n)
		}

		for _, pkg := range pkgs {
			for _, file := range pkg.Syntax {
				pos := pkg.Fset.Position(file.Pos())
				allCheckedFiles[filepath.ToSlash(pos.Filename)] = true
				if t.RepoRoot != "" {
					allCheckedFiles[relPath(t.RepoRoot, pos.Filename)] = true
				}
				if t.Dir != "" {
					allCheckedFiles[relPath(t.Dir, pos.Filename)] = true
				}
			}
		}

		graph, err := checker.Analyze(t.Analyzers, pkgs, nil)
		if err != nil {
			return fmt.Errorf("analyzers failed in %s: %w", t.Dir, err)
		}

		for _, act := range graph.Roots {
			if act.Err != nil {
				actionErrors = append(actionErrors, fmt.Errorf("%s: %w", act.Analyzer.Name, act.Err))
				continue
			}
			if !act.IsRoot {
				continue
			}
			if act.Analyzer.Name == "droppederrors" {
				if fList, ok := act.Result.([]droppederrors.Finding); ok {
					for _, f := range fList {
						k := f.Key()
						if seenFindings[k] {
							continue
						}
						seenFindings[k] = true
						allDroppedErrorsFindings = append(allDroppedErrorsFindings, f)
					}
				}
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
	}

	var errs []error
	for _, a := range t.Analyzers {
		if a.Name == "droppederrors" {
			if err := t.processDroppedErrors(a, allDroppedErrorsFindings, allCheckedFiles); err != nil {
				errs = append(errs, err)
			}
			continue
		}

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

func (t *AnalyzersTask) processDroppedErrors(a *analysis.Analyzer, findings []droppederrors.Finding, allCheckedFiles map[string]bool) error {
	baselineRoot := t.RepoRoot
	if baselineRoot == "" {
		baselineRoot = t.Dir
	}

	baselinePath := t.BaselinePath
	if baselinePath == "" {
		configured := ""
		if de, ok := t.Settings["droppederrors"].(*config.DroppedErrorsConfig); ok && de != nil {
			configured = de.Baseline
		}
		baselinePath = droppederrors.ResolveBaselinePath(t.RepoRoot, t.Dir, configured, t.WriteBaseline)
	}

	if baselinePath != "" {
		bDir := filepath.Dir(baselinePath)
		if filepath.Base(bDir) == ".ap" {
			baselineRoot = filepath.Dir(bDir)
		}
	}

	for i := range findings {
		findings[i].RelFile = relPath(baselineRoot, findings[i].Pos.Filename)
		findings[i].RepoRel = relPath(t.RepoRoot, findings[i].Pos.Filename)
		findings[i].ModRel = relPath(t.Dir, findings[i].Pos.Filename)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].RelFile != findings[j].RelFile {
			return findings[i].RelFile < findings[j].RelFile
		}
		if findings[i].Pos.Line != findings[j].Pos.Line {
			return findings[i].Pos.Line < findings[j].Pos.Line
		}
		if findings[i].Pos.Column != findings[j].Pos.Column {
			return findings[i].Pos.Column < findings[j].Pos.Column
		}
		if findings[i].Category != findings[j].Category {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].Callee < findings[j].Callee
	})

	if t.WriteBaseline {
		var finalEntries []droppederrors.BaselineEntry
		if baselinePath != "" {
			existingEntries, err := droppederrors.LoadBaseline(baselinePath)
			if err != nil {
				return err
			}
			for _, e := range existingEntries {
				entryFile := filepath.ToSlash(e.File)
				if !allCheckedFiles[entryFile] {
					finalEntries = append(finalEntries, e)
				}
			}
		}
		for _, f := range findings {
			relFile := relPath(baselineRoot, f.Pos.Filename)
			finalEntries = append(finalEntries, droppederrors.BaselineEntry{
				File:          relFile,
				EnclosingFunc: f.EnclosingFunc,
				Category:      f.Category,
				Callee:        f.Callee,
			})
		}
		if baselinePath != "" {
			if err := droppederrors.WriteBaseline(baselinePath, finalEntries); err != nil {
				return fmt.Errorf("failed to write baseline to %s: %w", baselinePath, err)
			}
		}
		fmt.Printf("Wrote %d baseline entries to %s\n", len(finalEntries), baselinePath)
		return nil
	}

	var baselineEntries []droppederrors.BaselineEntry
	if baselinePath != "" {
		entries, err := droppederrors.LoadBaseline(baselinePath)
		if err != nil {
			return err
		}
		baselineEntries = entries
	}

	newFindings, staleEntries := droppederrors.MatchFindings(findings, baselineEntries, allCheckedFiles, baselineRoot)
	for _, stale := range staleEntries {
		fmt.Printf("stale baseline entry: %s (remove from baseline)\n", stale)
	}

	findingsToReport := newFindings
	if len(baselineEntries) == 0 {
		findingsToReport = findings
	}

	for _, f := range findingsToReport {
		desc := "unchecked error"
		if f.Category == droppederrors.CategoryErrorOnlyCheckedForSuccess {
			desc = "error only checked for success"
		}
		fmt.Fprintf(os.Stderr, "%s:%d:%d: %s in %s: %s\n", f.Pos.Filename, f.Pos.Line, f.Pos.Column, desc, f.EnclosingFunc, f.Callee)
	}

	mode := t.Enabled[a.Name]
	if len(findingsToReport) > 0 {
		if mode == codestyle.ModeError {
			return fmt.Errorf("%s check failed in %s: %d new unchecked errors found", a.Name, t.Dir, len(findingsToReport))
		} else if mode == codestyle.ModeWarn {
			klog.Warningf("%s check failed in %s: %d diagnostics found", a.Name, t.Dir, len(findingsToReport))
		}
	}
	return nil
}

func relPath(base, target string) string {
	if base == "" {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}

func (t *AnalyzersTask) GetName() string {
	return "analyzers"
}

func (t *AnalyzersTask) GetChildren() []tasks.Task {
	return nil
}

func determineTargetGOOS(r *repo.Repo, dir string, skipGlobs []string, hostGOOS string) []string {
	if hostGOOS == "" {
		hostGOOS = runtime.GOOS
	}
	referenced := make(map[string]bool)

	var ignoreList *walker.IgnoreList
	if len(skipGlobs) > 0 {
		ignoreList = walker.NewIgnoreList(skipGlobs)
	}

	view := r.Under(dir)
	for _, f := range view.Files() {
		if !strings.HasSuffix(f.Name, ".go") {
			continue
		}
		if f.Module() != dir && f.Module() != "" {
			continue
		}
		if fileset.IsVendored(f.RelPath) || fileset.IsVendored(f.Path) {
			continue
		}
		if ignoreList != nil && ignoreList.ShouldIgnore(f.RelPath, false) {
			continue
		}
		for _, goos := range f.ReferencedGOOS() {
			if goos != hostGOOS {
				referenced[goos] = true
			}
		}
	}

	res := []string{hostGOOS}
	if len(referenced) > 0 {
		var nonHost []string
		for osTag := range referenced {
			nonHost = append(nonHost, osTag)
		}
		sort.Strings(nonHost)
		res = append(res, nonHost...)
	}
	return res
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
			Dir:      dir,
			RepoRoot: root,
			Enabled:  make(map[string]codestyle.Mode),
			Policies: make(map[string]fileset.Policy),
			Settings: make(map[string]config.LinterConfig),
		}

		if explicitGOOS := cfg.LintGOOS(); len(explicitGOOS) > 0 {
			analyzersTask.GOOS = explicitGOOS
		} else {
			analyzersTask.GOOS = determineTargetGOOS(r, dir, cfg.Skip, runtime.GOOS)
		}

		for _, reg := range codestyle.Analyzers {
			mode := reg.ResolveMode(cfg)
			if mode == codestyle.ModeIgnore {
				continue
			}
			analyzersTask.Enabled[reg.Name] = mode

			skipGen := reg.ResolveSkipGenerated(cfg)
			skipTests := reg.ResolveSkipTests(cfg)
			policy := fileset.NewPolicy(skipGen, skipTests, cfg.Skip, root)
			analyzersTask.Policies[reg.Name] = policy
			analyzersTask.Settings[reg.Name] = cfg.Linter(reg.ConfigKey())

			if reg.New != nil {
				analyzer := reg.New(policy, cfg)
				analyzersTask.Analyzers = append(analyzersTask.Analyzers, analyzer)
			}
		}

		if len(analyzersTask.Analyzers) > 0 {
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
	task, err := LintTasks(r, root)
	if err != nil {
		return err
	}
	return tasks.Run(ctx, &tasks.APScope{RepoRoot: root, Dir: root, Repo: r}, []tasks.Task{task}, tasks.RunOptions{})
}

// hasGoFiles returns true if the directory or any of its subdirectories
// (excluding those that are themselves Go modules) contain at least one .go file.
func hasGoFiles(r *repo.Repo, root string) (bool, error) {
	return r.Under(root).HasGoFiles(), nil
}
