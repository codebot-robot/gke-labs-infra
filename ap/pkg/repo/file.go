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

package repo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"sync"
	"time"
)

// File represents a file in the repository tree.
type File struct {
	Path    string // Absolute path on disk
	RelPath string // Path relative to view root (forward slash separated)
	Name    string // File base name

	apRoot string // Enclosing AP root directory path
	module string // Enclosing Go module directory path

	isGeneratedOnce sync.Once
	isGenerated     bool

	infoOnce sync.Once
	info     os.FileInfo
}

// Info returns an os.FileInfo representation of the file, resolved lazily.
func (f *File) Info() os.FileInfo {
	f.infoOnce.Do(func() {
		fi, err := os.Stat(f.Path)
		if err != nil {
			f.info = &fileInfo{name: f.Name}
			return
		}
		f.info = fi
	})
	return f.info
}

// APRoot returns the path to the closest enclosing AP root directory.
func (f *File) APRoot() string {
	return f.apRoot
}

// Module returns the path to the closest enclosing Go module root directory.
func (f *File) Module() string {
	return f.module
}

// IsTest reports whether this file is a Go test file.
func (f *File) IsTest() bool {
	return strings.HasSuffix(f.Name, "_test.go")
}

// IsGenerated reports whether this file is generated code (per https://go.dev/s/generatedcode).
func (f *File) IsGenerated() bool {
	if !strings.HasSuffix(f.Name, ".go") {
		return false
	}
	f.isGeneratedOnce.Do(func() {
		f.isGenerated = checkIsGenerated(f.Path)
	})
	return f.isGenerated
}

func checkIsGenerated(path string) bool {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		return false
	}
	return ast.IsGenerated(astFile)
}

type fileInfo struct {
	name string
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return 0 }
func (fi *fileInfo) Mode() os.FileMode  { return 0644 }
func (fi *fileInfo) ModTime() time.Time { return time.Time{} }
func (fi *fileInfo) IsDir() bool        { return false }
func (fi *fileInfo) Sys() any           { return nil }
