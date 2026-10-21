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

package klog

type Verbose bool

func V(level int) Verbose {
	return Verbose(true)
}

func (v Verbose) Info(args ...any)                    {}
func (v Verbose) Infof(format string, args ...any)    {}
func (v Verbose) Infoln(args ...any)                  {}
func (v Verbose) Warning(args ...any)                 {}
func (v Verbose) Warningf(format string, args ...any) {}
func (v Verbose) Error(args ...any)                   {}
func (v Verbose) Errorf(format string, args ...any)   {}

func Info(args ...any)                    {}
func Infof(format string, args ...any)    {}
func Infoln(args ...any)                  {}
func Warning(args ...any)                 {}
func Warningf(format string, args ...any) {}
func Warningln(args ...any)               {}
func Error(args ...any)                   {}
func Errorf(format string, args ...any)   {}
func Errorln(args ...any)                 {}
