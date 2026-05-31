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

package any_test

func foo() {
	var a interface{} // want "use any instead of interface{}"
	_ = a

	var b any
	_ = b

	var c map[string]interface{} // want "use any instead of interface{}"
	_ = c

	var d []interface{} // want "use any instead of interface{}"
	_ = d
}

type I interface { // OK
	Foo()
}

type J interface {
	interface{} // want "use any instead of interface{}"
}

func bar(i interface{}) {} // want "use any instead of interface{}"
