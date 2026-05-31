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

package fileheaders

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/walker"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"k8s.io/klog/v2"
)

var generatedCodeRegexp = regexp.MustCompile(`Code generated .* DO NOT EDIT`)

type FileHeadersOptions struct {
	IgnoreFiles []string `json:"ignore"`
}

func (o *FileHeadersOptions) InitDefaults() {
	o.IgnoreFiles = []string{
		".git/",
		".svn/",
		".hg/",
		"vendor/",
		"third_party/",
		"node_modules/",
	}
}

// processor handles file processing
type processor struct {
	config     *config.HeadersConfig
	ignoreList *walker.IgnoreList
}

func (p *processor) shouldIgnoreFile(relPath string, isDir bool) bool {
	return p.ignoreList.ShouldIgnore(relPath, isDir)
}

func Run(ctx context.Context, repoRoot string, files []string) error {
	var errs []error

	var opt FileHeadersOptions
	opt.InitDefaults()

	log := klog.FromContext(ctx)

	configFile := filepath.Join(repoRoot, ".ap/headers.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		klog.V(2).Info("No .ap/headers.yaml found, skipping file headers")
		return nil
	}

	config, err := config.LoadHeaders(repoRoot)
	if err != nil {
		return err
	}

	// Combine default ignores with config skips
	allIgnores := append(opt.IgnoreFiles, config.Skip...)
	ignoreList := walker.NewIgnoreList(allIgnores)

	processor := &processor{
		config:     config,
		ignoreList: ignoreList,
	}

	if len(files) == 0 {
		fv := walker.NewFileView(repoRoot, allIgnores)
		err := fv.Walk(func(f walker.File) error {
			// f.RelPath is already relative to repoRoot
			if err := processor.processFile(ctx, f.Path, f.RelPath); err != nil {
				log.Error(err, "Error processing file", "file", f.RelPath)
				// We don't abort walk on individual file error usually, but Walk signature expects error.
				// We should collect errors.
				errs = append(errs, fmt.Errorf("error processing %s: %w", f.RelPath, err))
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}
	} else {
		// Ensure we use absolute paths for IO, but relative paths for ignore checks.
		for _, file := range files {
			absPath := file
			if !filepath.IsAbs(file) {
				absPath = filepath.Join(repoRoot, file)
			}

			relPath, err := filepath.Rel(repoRoot, absPath)
			if err != nil {
				log.Error(err, "Skipping file outside repo root", "file", file)
				errs = append(errs, fmt.Errorf("skipping file outside repo root %s: %w", file, err))
				continue
			}

			if err := processor.processFile(ctx, absPath, relPath); err != nil {
				log.Error(err, "Error processing file", "file", file)
				errs = append(errs, fmt.Errorf("error processing %s: %w", file, err))
			}
		}
	}
	return errors.Join(errs...)
}

func makeExpectedHeaderRegex(style string, copyrightHolder string) (*regexp.Regexp, error) {
	s := regexp.QuoteMeta(style)
	holder := regexp.QuoteMeta(copyrightHolder)

	var patternParts []string
	patternParts = append(patternParts, s+`\s+Copyright\s+[0-9,\s-]+\s+`+holder)
	patternParts = append(patternParts, s)
	patternParts = append(patternParts, s+`\s+Licensed under the Apache License, Version 2.0 \(the "License"\);`)
	patternParts = append(patternParts, s+`\s+[yY]ou may not use this file except in compliance with the License\.`)
	patternParts = append(patternParts, s+`\s+[yY]ou may obtain a copy of the License at`)
	patternParts = append(patternParts, s)
	patternParts = append(patternParts, s+`\s+http://www\.apache\.org/licenses/LICENSE-2.0`)
	patternParts = append(patternParts, s)
	patternParts = append(patternParts, s+`\s+Unless required by applicable law or agreed to in writing, software`)
	patternParts = append(patternParts, s+`\s+distributed under the License is distributed on an "AS IS" BASIS,`)
	patternParts = append(patternParts, s+`\s+WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied\.`)
	patternParts = append(patternParts, s+`\s+See the License for the specific language governing permissions and`)
	patternParts = append(patternParts, s+`\s+limitations under the License\.`)

	pattern := `(?m)^\s*` + strings.Join(patternParts, `\s*(?:\r?\n)\s*`) + `\s*$`
	return regexp.Compile(pattern)
}

func (p *processor) processFile(ctx context.Context, absPath, relPath string) error {
	log := klog.FromContext(ctx)

	if p.shouldIgnoreFile(relPath, false) {
		return nil
	}

	ext := filepath.Ext(absPath)
	commentStyle := getCommentStyle(filepath.Base(absPath), ext)
	if commentStyle == "" {
		return nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}

	// Check for generated file
	// We check the first 2000 bytes to be efficient
	checkBuf := content
	if len(checkBuf) > 2000 {
		checkBuf = checkBuf[:2000]
	}

	if p.config.SkipGenerated != nil && *p.config.SkipGenerated {
		if generatedCodeRegexp.Match(checkBuf) {
			return nil
		}
	}

	// Check for K8s style block headers in Go files
	if ext == ".go" {
		// Look for /* ... Copyright ... */ pattern
		// We use a simplified regex that looks for /* followed by Copyright within the buffer
		if regexp.MustCompile(`(?s)/\*.*?Copyright`).Match(checkBuf) {
			return nil
		}
	}

	lines := strings.Split(string(content), "\n")

	// Find the start index for comments (skipping shebang and empty lines)
	startIdx := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		startIdx = 1
	}
	for startIdx < len(lines) && strings.TrimSpace(lines[startIdx]) == "" {
		startIdx++
	}

	// Now locate a potential copyright header block starting from startIdx
	var commentLines []string
	headerStartIdx := -1
	headerEndIdx := -1

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(commentLines) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, commentStyle) {
			if len(commentLines) == 0 {
				headerStartIdx = i
			}
			commentLines = append(commentLines, line)
			headerEndIdx = i + 1
		} else {
			if len(commentLines) > 0 {
				break
			}
			break
		}
	}

	joinedComments := ""
	if len(commentLines) > 0 {
		joinedComments = strings.Join(commentLines, "\n")
	}

	// Validate strictly against the contiguous leading comment block at the top of the file
	regex, err := makeExpectedHeaderRegex(commentStyle, p.config.CopyrightHolder)
	if err != nil {
		return err
	}

	if regex.MatchString(joinedComments) {
		return nil
	}

	log.Info("Updating or adding file header", "file", relPath)

	header, err := GenerateHeader(p.config, commentStyle)
	if err != nil {
		return err
	}

	foundHeaderBlock := false
	if len(commentLines) > 0 {
		if strings.Contains(strings.ToLower(joinedComments), "copyright") {
			foundHeaderBlock = true
		}
	}

	if !foundHeaderBlock {
		headerStartIdx = startIdx
		headerEndIdx = startIdx
	} else {
		// Replace the invalid copyright header block.
		// If there is an empty line immediately following the old header, skip it
		// because the generated header already ends with a trailing newline (blank line).
		if headerEndIdx < len(lines) && strings.TrimSpace(lines[headerEndIdx]) == "" {
			headerEndIdx++
		}
	}

	var newLines []string
	newLines = append(newLines, lines[:headerStartIdx]...)
	headerLines := strings.Split(header, "\n")
	newLines = append(newLines, headerLines...)
	newLines = append(newLines, lines[headerEndIdx:]...)

	output := strings.Join(newLines, "\n")
	return os.WriteFile(absPath, []byte(output), 0644)
}

func getCommentStyle(name, ext string) string {
	if name == "Dockerfile" {
		return "#"
	}
	switch ext {
	case ".go":
		return "//"
	case ".yaml", ".yml", ".sh", ".py", ".tf", ".toml":
		return "#"
	}
	return ""
}

func GenerateHeader(cfg *config.HeadersConfig, style string) (string, error) {
	year := time.Now().Year()

	if cfg.License != "apache-2.0" {
		return "", fmt.Errorf("unsupported license: %s", cfg.License)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s Copyright %d %s", style, year, cfg.CopyrightHolder))
	lines = append(lines, style)
	lines = append(lines, fmt.Sprintf("%s Licensed under the Apache License, Version 2.0 (the \"License\");", style))
	lines = append(lines, fmt.Sprintf("%s you may not use this file except in compliance with the License.", style))
	lines = append(lines, fmt.Sprintf("%s You may obtain a copy of the License at", style))
	lines = append(lines, style)
	lines = append(lines, fmt.Sprintf("%s     http://www.apache.org/licenses/LICENSE-2.0", style))
	lines = append(lines, style)
	lines = append(lines, fmt.Sprintf("%s Unless required by applicable law or agreed to in writing, software", style))
	lines = append(lines, fmt.Sprintf("%s distributed under the License is distributed on an \"AS IS\" BASIS,", style))
	lines = append(lines, fmt.Sprintf("%s WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.", style))
	lines = append(lines, fmt.Sprintf("%s See the License for the specific language governing permissions and", style))
	lines = append(lines, fmt.Sprintf("%s limitations under the License.", style))
	lines = append(lines, "")

	return strings.Join(lines, "\n"), nil
}
