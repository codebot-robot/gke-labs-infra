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

package goconst_test

import (
	"github.com/gke-labs/gke-labs-infra/experiments/goconst"
)

type Item struct {
	Value int
}

func takePointer(p *Item) {}

func takeConst(c goconst.Const[Item]) {}

func returnPointer(c goconst.Const[Item]) *Item {
	return c // want "implicit conversion from Const\\[T\\] to \\*T"
}

func returnConst(c goconst.Const[Item]) goconst.Const[Item] {
	return c // OK
}

func checkCases() {
	var item Item
	c := goconst.WrapConst(&item)

	// Safe: short declaration infers Const[Item]
	inferred := c
	_ = inferred

	// Safe: explicit conversion
	explicit := (*Item)(c)
	_ = explicit

	// Flag: implicit assignment in var decl
	var p1 *Item = c // want "implicit conversion from Const\\[T\\] to \\*T"
	_ = p1

	// Flag: implicit assignment
	var p2 *Item
	p2 = c // want "implicit conversion from Const\\[T\\] to \\*T"
	_ = p2

	// Flag: passing to func expecting *Item
	takePointer(c) // want "implicit conversion from Const\\[T\\] to \\*T"

	// Safe: passing to func expecting Const[Item]
	takeConst(c)

	// Flag: Struct literal field assignment
	type Container struct {
		Ptr *Item
	}
	_ = Container{
		Ptr: c, // want "implicit conversion from Const\\[T\\] to \\*T"
	}

	// Flag: Slice literal elements
	_ = []*Item{
		c, // want "implicit conversion from Const\\[T\\] to \\*T"
	}

	// Flag: Map literal value
	_ = map[string]*Item{
		"foo": c, // want "implicit conversion from Const\\[T\\] to \\*T"
	}

	// Flag: Map assignment
	m := make(map[string]*Item)
	m["foo"] = c // want "implicit conversion from Const\\[T\\] to \\*T"

	// Safe: map with Const[Item]
	m2 := make(map[string]goconst.Const[Item])
	m2["foo"] = c
}
