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

package a

import "errors"

func fail() error {
	return errors.New("fail")
}

func test() {
	_ = fail() // want "unchecked error"

	err := fail() // want "error only checked for success"
	if err == nil {
		println("ok")
	}

	if err := fail(); err != nil {
		_ = err // want "error discarded"
	}

	defer func() {
		if closeErr := fail(); closeErr != nil {
			_ = closeErr // want "error discarded"
		}
	}()
}

func testNegative() error {
	err := fail()
	_ = err // no finding
	return err
}
