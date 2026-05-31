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

package unused_test

type Const[T any] struct {
	val T
}

func (c *Const[T]) Read() T {
	return c.val
}

type UnusedStruct struct {
	field int // want "field field is unused"
}

func unusedFunc() { // want "func unusedFunc is unused"
}

func usedFunc() {
}

func Main() {
	usedFunc()
	c := &Const[int]{val: 1}
	_ = c.Read()
}
