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

package gostyle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/cache"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/walker"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
	"k8s.io/klog/v2"
)

var tidyRunner = func(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod tidy failed in %s: %w\n%s", dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Run(ctx context.Context, r *repo.Repo, repoRoot string, files []string) error {
	log := klog.FromContext(ctx)

	// Initialize cache
	cm, err := cache.NewManager()
	if err != nil {
		log.V(2).Info("Failed to initialize cache", "error", err)
	} else {
		defer func() {
			if err := cm.Save(); err != nil {
				log.Error(err, "Failed to save cache")
			}
		}()
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		return err
	}

	if cfg.IsGofmtEnabled() {
		if err := runGofmt(ctx, r, repoRoot, files, cfg.Skip, cm); err != nil {
			return err
		}
	}

	if cfg.IsTidyEnabled() {
		if err := runGoModTidy(ctx, r, repoRoot, files, cfg.Skip, cm); err != nil {
			return err
		}
	}

	return nil
}

func runGofmt(ctx context.Context, r *repo.Repo, repoRoot string, files []string, skip []string, cm *cache.Manager) error {
	log := klog.FromContext(ctx)

	ignoreList := walker.NewIgnoreList(append([]string{"vendor", ".git"}, skip...))
	expandedFiles, err := walker.ExpandPaths(r, repoRoot, files, ignoreList)
	if err != nil {
		return err
	}

	var filesToFormat []string
	for _, f := range expandedFiles {
		if strings.HasSuffix(f, ".go") {
			filesToFormat = append(filesToFormat, f)
		}
	}

	// Filter files using cache
	var dirtyFiles []string
	if cm != nil {
		for _, f := range filesToFormat {
			meta, err := cm.GetOrUpdateMetadata(f)
			if err != nil {
				dirtyFiles = append(dirtyFiles, f)
				continue
			}
			if !cm.IsGofmtDone(meta.Hash) {
				dirtyFiles = append(dirtyFiles, f)
			}
		}
	} else {
		dirtyFiles = filesToFormat
	}

	if len(dirtyFiles) == 0 {
		return nil
	}

	log.Info("Running gofmt", "files", len(dirtyFiles))

	// Chunk files to avoid argument length limits
	chunkSize := 100
	for i := 0; i < len(dirtyFiles); i += chunkSize {
		end := i + chunkSize
		if end > len(dirtyFiles) {
			end = len(dirtyFiles)
		}
		chunk := dirtyFiles[i:end]

		args := append([]string{"-w"}, chunk...)
		cmd := exec.CommandContext(ctx, "gofmt", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gofmt failed: %w", err)
		}
	}

	// Update cache for processed files
	if cm != nil {
		for _, f := range dirtyFiles {
			// Re-check metadata. gofmt might have changed it.
			meta, err := cm.GetOrUpdateMetadata(f)
			if err != nil {
				continue
			}
			cm.MarkGofmtDone(meta.Hash)
		}
	}

	return nil
}

func runGoModTidy(ctx context.Context, r *repo.Repo, repoRoot string, files []string, skip []string, cm *cache.Manager) error {
	log := klog.FromContext(ctx)

	ignoreList := walker.NewIgnoreList(append([]string{"vendor", ".git"}, skip...))
	candidateModules := findCandidateModules(r, repoRoot)

	for _, modDir := range candidateModules {
		relMod, err := filepath.Rel(repoRoot, modDir)
		if err != nil {
			continue
		}
		relSlash := filepath.ToSlash(relMod)
		if relSlash != "." {
			if ignoreList.ShouldIgnore(relSlash, true) || ignoreList.ShouldIgnore(filepath.ToSlash(filepath.Join(relMod, "go.mod")), false) {
				continue
			}
		}

		if !isModuleInScope(modDir, files, repoRoot, r) {
			continue
		}

		goFiles := getModuleGoFiles(r, modDir, repoRoot, ignoreList)

		// Check cache for unchanged module
		if cm != nil {
			if hash, ok := computeModuleHash(cm, modDir, repoRoot, goFiles); ok {
				if cm.IsTidyDone(hash) {
					log.V(2).Info("Module is tidy (cached)", "module", relMod)
					continue
				}
			}
		}

		log.Info("Running go mod tidy", "module", relMod)

		// In multi-module repositories or repositories with a go.work file, running
		// `go mod tidy` in workspace mode can cause dependencies to be resolved
		// across workspace siblings or lead to discrepancies between local workspace
		// builds and external/isolated builds. Disabling workspace mode with GOWORK=off
		// ensures that each module is tidied strictly on its own terms as an independent module.
		if err := tidyRunner(ctx, modDir); err != nil {
			return err
		}

		// Update cache after successful tidy
		if cm != nil {
			goModPath := filepath.Join(modDir, "go.mod")
			goSumPath := filepath.Join(modDir, "go.sum")
			if r != nil {
				if f := r.File(goModPath); f != nil {
					f.Refresh()
				}
				if f := r.File(goSumPath); f != nil {
					f.Refresh()
				}
			}

			// Refresh metadata for go.mod and go.sum
			if _, err := cm.GetOrUpdateMetadata(goModPath); err != nil {
				log.V(2).Info("failed to update metadata for go.mod", "error", err)
			}
			exists, err := fileutils.FileExists(goSumPath)
			if err != nil {
				log.V(2).Info("failed to check go.sum existence", "error", err)
			} else if exists {
				if _, err := cm.GetOrUpdateMetadata(goSumPath); err != nil {
					log.V(2).Info("failed to update metadata for go.sum", "error", err)
				}
			}

			if postHash, ok := computeModuleHash(cm, modDir, repoRoot, goFiles); ok {
				cm.MarkTidyDone(postHash)
			}
		}
	}

	return nil
}

func findCandidateModules(r *repo.Repo, repoRoot string) []string {
	seen := make(map[string]bool)
	var modules []string

	if r != nil {
		for _, m := range r.Modules() {
			cleanM := filepath.Clean(m)
			rel, err := filepath.Rel(repoRoot, cleanM)
			if err == nil && !strings.HasPrefix(rel, "..") {
				if !seen[cleanM] {
					seen[cleanM] = true
					modules = append(modules, cleanM)
				}
			}
		}
	}

	cleanRoot := filepath.Clean(repoRoot)
	if !seen[cleanRoot] {
		exists, err := fileutils.FileExists(filepath.Join(cleanRoot, "go.mod"))
		if err != nil {
			klog.V(2).Infof("failed to check go.mod in %s: %v", cleanRoot, err)
		} else if exists {
			seen[cleanRoot] = true
			modules = append(modules, cleanRoot)
		}
	}

	if r == nil {
		if err := filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && d.Name() == "go.mod" {
				dir := filepath.Dir(path)
				if !seen[dir] {
					seen[dir] = true
					modules = append(modules, dir)
				}
			}
			return nil
		}); err != nil {
			klog.V(2).Infof("failed to walk candidate modules in %s: %v", cleanRoot, err)
		}
	}

	sort.Strings(modules)
	return modules
}

func isModuleInScope(modDir string, files []string, repoRoot string, r *repo.Repo) bool {
	if len(files) == 0 {
		return true
	}
	cleanModDir := filepath.Clean(modDir)
	for _, f := range files {
		absF := f
		if !filepath.IsAbs(absF) {
			absF = filepath.Join(repoRoot, f)
		}
		absF = filepath.Clean(absF)

		if absF == cleanModDir || strings.HasPrefix(cleanModDir, absF+string(filepath.Separator)) {
			return true
		}
		if r != nil {
			if r.ModuleFor(absF) == cleanModDir {
				return true
			}
		} else {
			if strings.HasPrefix(absF, cleanModDir+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func getModuleGoFiles(r *repo.Repo, modDir, repoRoot string, ignoreList *walker.IgnoreList) []string {
	var goFiles []string
	cleanModDir := filepath.Clean(modDir)

	if r != nil {
		for _, f := range r.Under(cleanModDir).Files() {
			if strings.HasSuffix(f.Name, ".go") && f.Module() == cleanModDir {
				relF, err := filepath.Rel(repoRoot, f.Path)
				if err == nil && !ignoreList.ShouldIgnore(filepath.ToSlash(relF), false) {
					goFiles = append(goFiles, f.Path)
				}
			}
		}
	}

	if len(goFiles) == 0 {
		if err := filepath.WalkDir(cleanModDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != cleanModDir {
					exists, err := fileutils.FileExists(filepath.Join(path, "go.mod"))
					if err != nil {
						return err
					}
					if exists {
						return filepath.SkipDir
					}
				}
				relD, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return err
				}
				if ignoreList.ShouldIgnore(filepath.ToSlash(relD), true) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".go") {
				relF, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return err
				}
				if !ignoreList.ShouldIgnore(filepath.ToSlash(relF), false) {
					goFiles = append(goFiles, path)
				}
			}
			return nil
		}); err != nil {
			klog.V(2).Infof("failed to walk module go files in %s: %v", cleanModDir, err)
		}
	}

	sort.Strings(goFiles)
	return slices.Compact(goFiles)
}

func computeModuleHash(cm *cache.Manager, modDir, repoRoot string, goFiles []string) (string, bool) {
	if cm == nil {
		return "", false
	}

	goModPath := filepath.Join(modDir, "go.mod")
	modMeta, err := cm.GetOrUpdateMetadata(goModPath)
	if err != nil {
		return "", false
	}

	var sumMeta *cache.FileMetadata
	goSumPath := filepath.Join(modDir, "go.sum")
	exists, err := fileutils.FileExists(goSumPath)
	if err != nil {
		return "", false
	}
	if exists {
		sumMeta, err = cm.GetOrUpdateMetadata(goSumPath)
		if err != nil {
			return "", false
		}
	}

	h := sha256.New()
	relMod, err := filepath.Rel(repoRoot, modDir)
	if err != nil {
		return "", false
	}
	h.Write([]byte(filepath.ToSlash(relMod) + "\n"))
	h.Write([]byte("go.mod:" + modMeta.Hash + "\n"))
	if sumMeta != nil {
		h.Write([]byte("go.sum:" + sumMeta.Hash + "\n"))
	} else {
		h.Write([]byte("go.sum:none\n"))
	}

	for _, gf := range goFiles {
		meta, err := cm.GetOrUpdateMetadata(gf)
		if err != nil {
			return "", false
		}
		rel, err := filepath.Rel(modDir, gf)
		if err != nil {
			return "", false
		}
		h.Write([]byte(filepath.ToSlash(rel) + ":" + meta.Hash + "\n"))
	}

	return hex.EncodeToString(h.Sum(nil)), true
}
