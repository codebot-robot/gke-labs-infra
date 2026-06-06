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

package goconst

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "goconst",
	Doc:  "check for implicit conversions from Const[T] to *T",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if isGenerated(f) {
			continue
		}
		ast.Walk(&visitor{pass: pass}, f)
	}
	return nil, nil
}

func isGenerated(f *ast.File) bool {
	for _, comment := range f.Comments {
		for _, c := range comment.List {
			if strings.Contains(c.Text, "Code generated") || strings.Contains(c.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

type visitor struct {
	pass       *analysis.Pass
	currentSig *types.Signature
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	nextSig := v.currentSig
	switch n := node.(type) {
	case *ast.FuncDecl:
		if obj, ok := v.pass.TypesInfo.Defs[n.Name].(*types.Func); ok {
			if sig, ok := obj.Type().(*types.Signature); ok {
				nextSig = sig
			}
		}
	case *ast.FuncLit:
		if sig, ok := v.pass.TypesInfo.TypeOf(n).(*types.Signature); ok {
			nextSig = sig
		}
	case *ast.ReturnStmt:
		if v.currentSig != nil && len(n.Results) > 0 {
			results := v.currentSig.Results()
			if len(n.Results) == results.Len() {
				for i := range n.Results {
					checkConv(n.Results[i], results.At(i).Type(), v.pass)
				}
			} else if len(n.Results) == 1 {
				rhsTypes := getMultiValueTypes(n.Results[0], v.pass)
				for i := 0; i < results.Len() && i < len(rhsTypes); i++ {
					valType := rhsTypes[i]
					retType := results.At(i).Type()
					if isConstType(valType) && !isConstType(retType) {
						if types.Identical(valType.Underlying(), retType.Underlying()) {
							v.pass.Reportf(n.Results[0].Pos(), "implicit conversion from Const[T] to *T")
						}
					}
				}
			}
		}
	}

	checkNode(node, v.pass)

	return &visitor{
		pass:       v.pass,
		currentSig: nextSig,
	}
}

func isConstType(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj != nil && obj.Pkg() != nil {
			return obj.Pkg().Path() == "github.com/gke-labs/gke-labs-infra/experiments/goconst" && obj.Name() == "Const"
		}
	}
	return false
}

func checkConv(expr ast.Expr, targetType types.Type, pass *analysis.Pass) {
	if expr == nil || targetType == nil {
		return
	}
	exprType := pass.TypesInfo.TypeOf(expr)
	if exprType == nil {
		return
	}

	if isConstType(exprType) && !isConstType(targetType) {
		if types.Identical(exprType.Underlying(), targetType.Underlying()) {
			pass.Reportf(expr.Pos(), "implicit conversion from Const[T] to *T")
		}
	}
}

func getMultiValueTypes(expr ast.Expr, pass *analysis.Pass) []types.Type {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return nil
	}
	if tuple, ok := t.(*types.Tuple); ok {
		res := make([]types.Type, tuple.Len())
		for i := 0; i < tuple.Len(); i++ {
			res[i] = tuple.At(i).Type()
		}
		return res
	}
	switch e := expr.(type) {
	case *ast.IndexExpr:
		if mapType, ok := pass.TypesInfo.TypeOf(e.X).Underlying().(*types.Map); ok {
			return []types.Type{mapType.Elem(), types.Typ[types.Bool]}
		}
	case *ast.UnaryExpr:
		if e.Op == token.ARROW {
			if chanType, ok := pass.TypesInfo.TypeOf(e.X).Underlying().(*types.Chan); ok {
				return []types.Type{chanType.Elem(), types.Typ[types.Bool]}
			}
		}
	case *ast.TypeAssertExpr:
		return []types.Type{pass.TypesInfo.TypeOf(e.Type), types.Typ[types.Bool]}
	}
	return []types.Type{t}
}

func checkNode(node ast.Node, pass *analysis.Pass) {
	switch n := node.(type) {
	case *ast.AssignStmt:
		if len(n.Lhs) == len(n.Rhs) {
			for i := range n.Lhs {
				lhsType := pass.TypesInfo.TypeOf(n.Lhs[i])
				checkConv(n.Rhs[i], lhsType, pass)
			}
		} else if len(n.Rhs) == 1 {
			rhsTypes := getMultiValueTypes(n.Rhs[0], pass)
			for i := 0; i < len(n.Lhs) && i < len(rhsTypes); i++ {
				lhsType := pass.TypesInfo.TypeOf(n.Lhs[i])
				if lhsType != nil && rhsTypes[i] != nil {
					valType := rhsTypes[i]
					if isConstType(valType) && !isConstType(lhsType) {
						if types.Identical(valType.Underlying(), lhsType.Underlying()) {
							pass.Reportf(n.Rhs[0].Pos(), "implicit conversion from Const[T] to *T")
						}
					}
				}
			}
		}

	case *ast.ValueSpec:
		if len(n.Values) == 0 {
			return
		}
		if len(n.Names) == len(n.Values) {
			for i := range n.Names {
				lhsType := pass.TypesInfo.TypeOf(n.Names[i])
				checkConv(n.Values[i], lhsType, pass)
			}
		} else if len(n.Values) == 1 {
			rhsTypes := getMultiValueTypes(n.Values[0], pass)
			for i := 0; i < len(n.Names) && i < len(rhsTypes); i++ {
				lhsType := pass.TypesInfo.TypeOf(n.Names[i])
				if lhsType != nil && rhsTypes[i] != nil {
					valType := rhsTypes[i]
					if isConstType(valType) && !isConstType(lhsType) {
						if types.Identical(valType.Underlying(), lhsType.Underlying()) {
							pass.Reportf(n.Values[0].Pos(), "implicit conversion from Const[T] to *T")
						}
					}
				}
			}
		}

	case *ast.CallExpr:
		if pass.TypesInfo.Types[n.Fun].IsType() {
			// Explicit type conversion, skip
			return
		}
		funType := pass.TypesInfo.TypeOf(n.Fun)
		if sig, ok := funType.(*types.Signature); ok {
			params := sig.Params()
			for i, arg := range n.Args {
				var paramType types.Type
				if sig.Variadic() && i >= params.Len()-1 {
					lastParam := params.At(params.Len() - 1)
					if sliceType, ok := lastParam.Type().(*types.Slice); ok {
						paramType = sliceType.Elem()
					}
				} else if i < params.Len() {
					paramType = params.At(i).Type()
				}
				if paramType != nil {
					checkConv(arg, paramType, pass)
				}
			}
		}

	case *ast.SendStmt:
		chanType := pass.TypesInfo.TypeOf(n.Chan)
		if chanType != nil {
			if ch, ok := chanType.Underlying().(*types.Chan); ok {
				checkConv(n.Value, ch.Elem(), pass)
			}
		}

	case *ast.CompositeLit:
		litType := pass.TypesInfo.TypeOf(n)
		if litType == nil {
			return
		}
		underlying := litType.Underlying()
		switch cl := underlying.(type) {
		case *types.Struct:
			for i, elt := range n.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if ident, ok := kv.Key.(*ast.Ident); ok {
						if fieldVar, ok := pass.TypesInfo.Uses[ident].(*types.Var); ok {
							checkConv(kv.Value, fieldVar.Type(), pass)
						}
					}
				} else {
					if i < cl.NumFields() {
						checkConv(elt, cl.Field(i).Type(), pass)
					}
				}
			}
		case *types.Map:
			for _, elt := range n.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					checkConv(kv.Key, cl.Key(), pass)
					checkConv(kv.Value, cl.Elem(), pass)
				}
			}
		case *types.Slice:
			for _, elt := range n.Elts {
				checkConv(elt, cl.Elem(), pass)
			}
		case *types.Array:
			for _, elt := range n.Elts {
				checkConv(elt, cl.Elem(), pass)
			}
		}
	}
}
