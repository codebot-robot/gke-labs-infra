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

type closer interface {
	Close() error
}

func testForBody(f closer) {
	for i := 0; i < 10; i++ {
		defer f.Close() // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
	}
}

func testRangeBody(f closer, items []int) {
	for range items {
		defer f.Close() // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
	}
}

func testLoopWithFuncLit(f closer, items []int) {
	for range items {
		func() {
			defer f.Close()
		}()
	}
}

func testDeferTopLevel(f closer) {
	defer f.Close()

	for i := 0; i < 10; i++ {
		// loop without defer
	}
}

func testDeferFuncLitInLoop(f closer) {
	for i := 0; i < 5; i++ {
		defer func() { // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
			f.Close()
		}()
	}
}

func testNestedLoops(f closer) {
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			defer f.Close() // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
		}
	}
}

func testDeferInIfInLoop(f closer) {
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			defer f.Close() // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
		}
	}
}

func testLoopInFuncLitInLoop(f closer) {
	for i := 0; i < 5; i++ {
		func() {
			for j := 0; j < 5; j++ {
				defer f.Close() // want `defer inside a loop runs when the function returns, not each iteration; move the body into a function`
			}
		}()
	}
}
