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

package droppederrors

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
)

// DefaultExcludedSymbols lists functions and methods whose error is ignored by default
// because their errors are meaningless by contract (e.g. bytes.Buffer.Write never fails).
var DefaultExcludedSymbols = []string{
	// bytes
	"(*bytes.Buffer).Write",
	"(*bytes.Buffer).WriteByte",
	"(*bytes.Buffer).WriteRune",
	"(*bytes.Buffer).WriteString",

	// crypto/rand
	"crypto/rand.Read",

	// fmt
	"fmt.Print",
	"fmt.Printf",
	"fmt.Println",
	"fmt.Fprint(*bytes.Buffer)",
	"fmt.Fprintf(*bytes.Buffer)",
	"fmt.Fprintln(*bytes.Buffer)",
	"fmt.Fprint(*strings.Builder)",
	"fmt.Fprintf(*strings.Builder)",
	"fmt.Fprintln(*strings.Builder)",
	"fmt.Fprint(os.Stderr)",
	"fmt.Fprintf(os.Stderr)",
	"fmt.Fprintln(os.Stderr)",

	// io
	"(*io.PipeReader).CloseWithError",
	"(*io.PipeWriter).CloseWithError",

	// math/rand
	"math/rand.Read",
	"(*math/rand.Rand).Read",

	// strings
	"(*strings.Builder).Write",
	"(*strings.Builder).WriteByte",
	"(*strings.Builder).WriteRune",
	"(*strings.Builder).WriteString",

	// hash
	"(hash.Hash).Write",
	"(*crypto/sha3.SHA3).Write",
	"(*crypto/sha3.SHAKE).Read",
	"(*crypto/sha3.SHAKE).Write",

	// hash/maphash
	"(*hash/maphash.Hash).Write",
	"(*hash/maphash.Hash).WriteByte",
	"(*hash/maphash.Hash).WriteString",
}

// ExclusionMatcher checks whether a function/method call matches any exclusion symbol.
type ExclusionMatcher struct {
	symbols map[string]bool
}

// NewExclusionMatcher constructs an ExclusionMatcher with default and custom symbols.
func NewExclusionMatcher(customSymbols []string, useDefaultExcludes bool) *ExclusionMatcher {
	m := &ExclusionMatcher{
		symbols: make(map[string]bool),
	}
	if useDefaultExcludes {
		for _, sym := range DefaultExcludedSymbols {
			m.addSymbol(sym)
		}
	}
	for _, sym := range customSymbols {
		m.addSymbol(sym)
	}
	return m
}

func (m *ExclusionMatcher) addSymbol(sym string) {
	sym = strings.TrimSpace(sym)
	if sym == "" {
		return
	}
	m.symbols[sym] = true

	// If symbol has no receiver parens: "Type.Method", also add "(*Type).Method" and "(Type).Method"
	if !strings.HasPrefix(sym, "(") && strings.Contains(sym, ".") {
		lastDot := strings.LastIndex(sym, ".")
		typePart := sym[:lastDot]
		methodPart := sym[lastDot+1:]
		m.symbols[fmt.Sprintf("(*%s).%s", typePart, methodPart)] = true
		m.symbols[fmt.Sprintf("(%s).%s", typePart, methodPart)] = true
	}
}

// IsExcluded checks if call is excluded.
func (m *ExclusionMatcher) IsExcluded(call *ast.CallExpr, info *types.Info) bool {
	if call == nil || info == nil {
		return false
	}

	names := m.namesForCall(call, info)
	arg0 := m.arg0String(call, info)

	for _, name := range names {
		if m.symbols[name] {
			return true
		}
		if arg0 != "" && m.symbols[name+"("+arg0+")"] {
			return true
		}
	}

	return false
}

func (m *ExclusionMatcher) namesForCall(call *ast.CallExpr, info *types.Info) []string {
	var names []string

	// Selector: obj.Method or pkg.Func
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if selection, ok := info.Selections[sel]; ok {
			fn, ok := selection.Obj().(*types.Func)
			if ok {
				names = append(names, fn.FullName())
				// Recv type qualified names
				recv := selection.Recv()
				if recv != nil {
					names = append(names, fmt.Sprintf("(%s).%s", recv.String(), fn.Name()))
					names = append(names, fmt.Sprintf("(*%s).%s", strings.TrimPrefix(recv.String(), "*"), fn.Name()))

					// Walk embedded interfaces of the receiver type
					walkEmbeddedInterfaces(recv, fn.Name(), &names)
				}
				names = append(names, fn.Name())
				if fn.Pkg() != nil {
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Name(), fn.Name()))
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Path(), fn.Name()))
				}
			}
		} else if obj, ok := info.Uses[sel.Sel]; ok {
			if fn, ok := obj.(*types.Func); ok {
				names = append(names, fn.FullName())
				if fn.Pkg() != nil {
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Name(), fn.Name()))
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Path(), fn.Name()))
				}
				names = append(names, fn.Name())
			}
		}
		names = append(names, types.ExprString(sel))
		return names
	}

	// Ident: local function call
	if id, ok := call.Fun.(*ast.Ident); ok {
		if obj, ok := info.Uses[id]; ok {
			if fn, ok := obj.(*types.Func); ok {
				names = append(names, fn.FullName())
				if fn.Pkg() != nil {
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Name(), fn.Name()))
					names = append(names, fmt.Sprintf("%s.%s", fn.Pkg().Path(), fn.Name()))
				}
			}
		}
		names = append(names, id.Name)
	}

	return names
}

func walkEmbeddedInterfaces(t types.Type, methodName string, out *[]string) {
	if t == nil {
		return
	}
	// If pointer, get element
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	visited := make(map[types.Type]bool)
	var walk func(types.Type)
	walk = func(curr types.Type) {
		if curr == nil || visited[curr] {
			return
		}
		visited[curr] = true

		iface, ok := curr.Underlying().(*types.Interface)
		if !ok {
			return
		}

		for i := 0; i < iface.NumEmbeddeds(); i++ {
			emb := iface.EmbeddedType(i)
			if hasMethod(emb, methodName) {
				*out = append(*out, fmt.Sprintf("(%s).%s", emb.String(), methodName))
			}
			walk(emb)
		}
	}
	walk(t)
}

func hasMethod(t types.Type, name string) bool {
	if iface, ok := t.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			if iface.Method(i).Name() == name {
				return true
			}
		}
	}
	return false
}

func (m *ExclusionMatcher) arg0String(call *ast.CallExpr, info *types.Info) string {
	if len(call.Args) == 0 {
		return ""
	}
	arg := call.Args[0]

	// 1. Check if argument expression is e.g. os.Stderr
	exprStr := types.ExprString(arg)
	if exprStr == "os.Stderr" || exprStr == "os.Stdout" || exprStr == "os.Stdin" {
		return exprStr
	}

	// 2. Exact type comparison for *bytes.Buffer and *strings.Builder
	tv, ok := info.Types[arg]
	if ok && tv.Type != nil {
		if isExactNamedPointerType(tv.Type, "bytes", "Buffer") {
			return "*bytes.Buffer"
		}
		if isExactNamedPointerType(tv.Type, "strings", "Builder") {
			return "*strings.Builder"
		}
		return tv.Type.String()
	}

	return exprStr
}

func isExactNamedPointerType(t types.Type, pkgPath, typeName string) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == pkgPath && named.Obj().Name() == typeName
}
