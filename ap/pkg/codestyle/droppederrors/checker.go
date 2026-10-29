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
	"go/ast"
	"go/token"
	"go/types"
)

var (
	errorType      = types.Universe.Lookup("error").Type()
	errorInterface = errorType.Underlying().(*types.Interface)
)

func isErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	return types.Implements(t, errorInterface)
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func buildParentMap(file *ast.File) map[ast.Node]ast.Node {
	parentMap := make(map[ast.Node]ast.Node)
	var walkParents func(ast.Node, ast.Node)
	walkParents = func(curr, parent ast.Node) {
		if curr == nil {
			return
		}
		if parent != nil {
			parentMap[curr] = parent
		}
		ast.Inspect(curr, func(child ast.Node) bool {
			if child != nil && child != curr {
				walkParents(child, curr)
				return false
			}
			return true
		})
	}
	walkParents(file, nil)
	return parentMap
}

func isNilComparisonOperand(id *ast.Ident, parentMap map[ast.Node]ast.Node) bool {
	curr := ast.Expr(id)
	parent := parentMap[curr]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		curr = paren
		parent = parentMap[curr]
	}
	bin, ok := parent.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op != token.EQL && bin.Op != token.NEQ {
		return false
	}
	if unparen(bin.X) == id && isNilExpr(unparen(bin.Y)) {
		return true
	}
	if unparen(bin.Y) == id && isNilExpr(unparen(bin.X)) {
		return true
	}
	return false
}

func isBlankAssignRHS(id *ast.Ident, parentMap map[ast.Node]ast.Node) bool {
	curr := ast.Expr(id)
	parent := parentMap[curr]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		curr = paren
		parent = parentMap[curr]
	}
	switch stmt := parent.(type) {
	case *ast.AssignStmt:
		if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
			if lhsIdent, ok := stmt.Lhs[0].(*ast.Ident); ok && lhsIdent.Name == "_" {
				return unparen(stmt.Rhs[0]) == id
			}
		} else if len(stmt.Lhs) == len(stmt.Rhs) {
			for i := range stmt.Rhs {
				if unparen(stmt.Rhs[i]) == id {
					if lhsIdent, ok := stmt.Lhs[i].(*ast.Ident); ok && lhsIdent.Name == "_" {
						return true
					}
				}
			}
		}
	case *ast.ValueSpec:
		if len(stmt.Names) == 1 && len(stmt.Values) == 1 {
			if stmt.Names[0].Name == "_" {
				return unparen(stmt.Values[0]) == id
			}
		} else if len(stmt.Names) == len(stmt.Values) {
			for i := range stmt.Values {
				if unparen(stmt.Values[i]) == id {
					if stmt.Names[i].Name == "_" {
						return true
					}
				}
			}
		}
	}
	return false
}

func isAssignLHS(id *ast.Ident, parentMap map[ast.Node]ast.Node) bool {
	curr := ast.Expr(id)
	parent := parentMap[curr]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		curr = paren
		parent = parentMap[curr]
	}
	assign, ok := parent.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, lhs := range assign.Lhs {
		if unparen(lhs) == id {
			return true
		}
	}
	return false
}

func isDiscardedErrorVar(rhsIdent *ast.Ident, info *types.Info, parentMap map[ast.Node]ast.Node) bool {
	if rhsIdent == nil || rhsIdent.Name == "_" || rhsIdent.Name == "nil" || info == nil || parentMap == nil {
		return false
	}
	obj, ok := info.Uses[rhsIdent]
	if !ok {
		return false
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return false
	}
	if !isErrorType(v.Type()) {
		return false
	}
	// Only flag local variables and function parameters, not package-level variables
	if v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
		return false
	}

	// Count uses of the variable object; skip if any use is not a nil comparison,
	// a blank assignment right-hand side, or an assignment left-hand side.
	for id, usedObj := range info.Uses {
		if usedObj != v {
			continue
		}
		if isNilComparisonOperand(id, parentMap) {
			continue
		}
		if isBlankAssignRHS(id, parentMap) {
			continue
		}
		if isAssignLHS(id, parentMap) {
			continue
		}
		return false
	}
	return true
}

func isTypeConversion(info *types.Info, call *ast.CallExpr) bool {
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
		return true
	}
	return false
}

func isRecoverCall(call *ast.CallExpr, info *types.Info) bool {
	if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "recover" {
		if obj, ok := info.Uses[id]; ok {
			if b, ok := obj.(*types.Builtin); ok && b.Name() == "recover" {
				return true
			}
		}
	}
	return false
}

func returnErrorIndices(info *types.Info, call *ast.CallExpr) []bool {
	if isTypeConversion(info, call) {
		return nil
	}
	tv, ok := info.Types[call]
	if !ok || tv.Type == nil {
		return nil
	}
	if tuple, ok := tv.Type.(*types.Tuple); ok {
		res := make([]bool, tuple.Len())
		hasErr := false
		for i := 0; i < tuple.Len(); i++ {
			if isErrorType(tuple.At(i).Type()) {
				res[i] = true
				hasErr = true
			}
		}
		if !hasErr {
			return nil
		}
		return res
	}
	if isErrorType(tv.Type) {
		return []bool{true}
	}
	return nil
}

type rawFinding struct {
	pos      token.Pos
	expr     ast.Expr // *ast.CallExpr or other expression
	category string
}

func findDroppedErrorsInFile(file *ast.File, info *types.Info, matcher *ExclusionMatcher) []rawFinding {
	parentMap := buildParentMap(file)
	var findings []rawFinding

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		switch stmt := n.(type) {
		case *ast.ExprStmt:
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				errIndices := returnErrorIndices(info, call)
				if len(errIndices) > 0 && !matcher.IsExcluded(call, info) {
					findings = append(findings, rawFinding{pos: call.Pos(), expr: call, category: CategoryDroppedError})
				}
			}

		case *ast.GoStmt:
			errIndices := returnErrorIndices(info, stmt.Call)
			if len(errIndices) > 0 && !matcher.IsExcluded(stmt.Call, info) {
				findings = append(findings, rawFinding{pos: stmt.Call.Pos(), expr: stmt.Call, category: CategoryDroppedError})
			}

		case *ast.DeferStmt:
			errIndices := returnErrorIndices(info, stmt.Call)
			if len(errIndices) > 0 && !matcher.IsExcluded(stmt.Call, info) {
				findings = append(findings, rawFinding{pos: stmt.Call.Pos(), expr: stmt.Call, category: CategoryDroppedError})
			}

		case *ast.AssignStmt:
			if len(stmt.Rhs) == 1 {
				if call, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
					isRecv := isRecoverCall(call, info)
					errIndices := returnErrorIndices(info, call)
					if (len(errIndices) > 0 || isRecv) && !matcher.IsExcluded(call, info) {
						for i := 0; i < len(stmt.Lhs); i++ {
							if isRecv || (i < len(errIndices) && errIndices[i]) {
								if id, ok := stmt.Lhs[i].(*ast.Ident); ok && id.Name == "_" {
									findings = append(findings, rawFinding{pos: id.Pos(), expr: call, category: CategoryDroppedError})
								}
							}
						}
					}
				} else if len(stmt.Lhs) == 1 {
					if id, ok := stmt.Lhs[0].(*ast.Ident); ok && id.Name == "_" {
						if rhsIdent, ok := unparen(stmt.Rhs[0]).(*ast.Ident); ok {
							if isDiscardedErrorVar(rhsIdent, info, parentMap) {
								findings = append(findings, rawFinding{pos: id.Pos(), expr: rhsIdent, category: CategoryErrorDiscarded})
							}
						}
					}
				}
			} else if len(stmt.Lhs) == len(stmt.Rhs) {
				for i := range stmt.Lhs {
					if id, ok := stmt.Lhs[i].(*ast.Ident); ok && id.Name == "_" {
						if call, ok := stmt.Rhs[i].(*ast.CallExpr); ok {
							isRecv := isRecoverCall(call, info)
							errIndices := returnErrorIndices(info, call)
							if (isRecv || (len(errIndices) > 0 && errIndices[0])) && !matcher.IsExcluded(call, info) {
								findings = append(findings, rawFinding{pos: id.Pos(), expr: call, category: CategoryDroppedError})
							}
						} else if rhsIdent, ok := unparen(stmt.Rhs[i]).(*ast.Ident); ok {
							if isDiscardedErrorVar(rhsIdent, info, parentMap) {
								findings = append(findings, rawFinding{pos: id.Pos(), expr: rhsIdent, category: CategoryErrorDiscarded})
							}
						}
					}
				}
			}

		case *ast.ValueSpec:
			if len(stmt.Values) == 1 {
				if call, ok := stmt.Values[0].(*ast.CallExpr); ok {
					isRecv := isRecoverCall(call, info)
					errIndices := returnErrorIndices(info, call)
					if (len(errIndices) > 0 || isRecv) && !matcher.IsExcluded(call, info) {
						for i := 0; i < len(stmt.Names); i++ {
							if isRecv || (i < len(errIndices) && errIndices[i]) {
								if stmt.Names[i].Name == "_" {
									findings = append(findings, rawFinding{pos: stmt.Names[i].Pos(), expr: call, category: CategoryDroppedError})
								}
							}
						}
					}
				} else if len(stmt.Names) == 1 {
					if stmt.Names[0].Name == "_" {
						if rhsIdent, ok := unparen(stmt.Values[0]).(*ast.Ident); ok {
							if isDiscardedErrorVar(rhsIdent, info, parentMap) {
								findings = append(findings, rawFinding{pos: stmt.Names[0].Pos(), expr: rhsIdent, category: CategoryErrorDiscarded})
							}
						}
					}
				}
			} else if len(stmt.Names) == len(stmt.Values) {
				for i := range stmt.Names {
					if stmt.Names[i].Name == "_" {
						if call, ok := stmt.Values[i].(*ast.CallExpr); ok {
							isRecv := isRecoverCall(call, info)
							errIndices := returnErrorIndices(info, call)
							if (isRecv || (len(errIndices) > 0 && errIndices[0])) && !matcher.IsExcluded(call, info) {
								findings = append(findings, rawFinding{pos: stmt.Names[i].Pos(), expr: call, category: CategoryDroppedError})
							}
						} else if rhsIdent, ok := unparen(stmt.Values[i]).(*ast.Ident); ok {
							if isDiscardedErrorVar(rhsIdent, info, parentMap) {
								findings = append(findings, rawFinding{pos: stmt.Names[i].Pos(), expr: rhsIdent, category: CategoryErrorDiscarded})
							}
						}
					}
				}
			}

		case *ast.RangeStmt:
			checkRangeStmt(stmt, info, matcher, &findings)
		}

		return true
	})

	// Check for "error only tested for success" pattern
	successOnlyFindings := findSuccessOnlyChecks(file, info, parentMap)
	findings = append(findings, successOnlyFindings...)

	return findings
}

func checkRangeStmt(stmt *ast.RangeStmt, info *types.Info, matcher *ExclusionMatcher, findings *[]rawFinding) {
	if stmt == nil || stmt.X == nil || info == nil {
		return
	}
	tv, ok := info.Types[stmt.X]
	if !ok || tv.Type == nil {
		return
	}
	sig, ok := tv.Type.Underlying().(*types.Signature)
	if !ok || sig.Params().Len() != 1 {
		return
	}
	yieldSig, ok := sig.Params().At(0).Type().Underlying().(*types.Signature)
	if !ok {
		return
	}

	// Seq2: yield(K, V) where V is error
	if yieldSig.Params().Len() == 2 {
		valType := yieldSig.Params().At(1).Type()
		if isErrorType(valType) {
			dropped := false
			if stmt.Value == nil {
				// for k := range seq
				dropped = true
			} else if id, ok := stmt.Value.(*ast.Ident); ok && id.Name == "_" {
				// for k, _ := range seq
				dropped = true
			}
			if dropped {
				*findings = append(*findings, rawFinding{
					pos:      stmt.Pos(),
					expr:     stmt.X,
					category: CategoryDroppedError,
				})
			}
		}
	} else if yieldSig.Params().Len() == 1 {
		// Seq: yield(V) where V is error
		valType := yieldSig.Params().At(0).Type()
		if isErrorType(valType) {
			dropped := false
			if stmt.Key == nil {
				// for range seq
				dropped = true
			} else if id, ok := stmt.Key.(*ast.Ident); ok && id.Name == "_" {
				// for _ := range seq
				dropped = true
			}
			if dropped {
				*findings = append(*findings, rawFinding{
					pos:      stmt.Pos(),
					expr:     stmt.X,
					category: CategoryDroppedError,
				})
			}
		}
	}
}

type errVarDef struct {
	obj  *types.Var
	call *ast.CallExpr
	pos  token.Pos
}

func findSuccessOnlyChecks(file *ast.File, info *types.Info, parentMap map[ast.Node]ast.Node) []rawFinding {
	var errDefs []errVarDef

	// 1. Identify local error variables whose value comes from a call
	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok == token.DEFINE || stmt.Tok == token.ASSIGN {
				if len(stmt.Rhs) == 1 {
					if call, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
						errIndices := returnErrorIndices(info, call)
						for i := range stmt.Lhs {
							if i < len(errIndices) && errIndices[i] {
								if id, ok := stmt.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
									if v, ok := getVarObj(info, id); ok && isErrorType(v.Type()) {
										errDefs = append(errDefs, errVarDef{obj: v, call: call, pos: id.Pos()})
									}
								}
							}
						}
					}
				} else if len(stmt.Lhs) == len(stmt.Rhs) {
					for i := range stmt.Lhs {
						if id, ok := stmt.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
							if call, ok := stmt.Rhs[i].(*ast.CallExpr); ok {
								errIndices := returnErrorIndices(info, call)
								if len(errIndices) > 0 && errIndices[0] {
									if v, ok := getVarObj(info, id); ok && isErrorType(v.Type()) {
										errDefs = append(errDefs, errVarDef{obj: v, call: call, pos: id.Pos()})
									}
								}
							}
						}
					}
				}
			}
		case *ast.ValueSpec:
			if len(stmt.Values) == 1 {
				if call, ok := stmt.Values[0].(*ast.CallExpr); ok {
					errIndices := returnErrorIndices(info, call)
					for i := range stmt.Names {
						if i < len(errIndices) && errIndices[i] && stmt.Names[i].Name != "_" {
							if v, ok := getVarObj(info, stmt.Names[i]); ok && isErrorType(v.Type()) {
								errDefs = append(errDefs, errVarDef{obj: v, call: call, pos: stmt.Names[i].Pos()})
							}
						}
					}
				}
			}
		}
		return true
	})

	if len(errDefs) == 0 {
		return nil
	}

	var findings []rawFinding

	// 3. For each error variable, inspect all uses
	for _, def := range errDefs {
		var uses []*ast.Ident
		ast.Inspect(file, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if info.Uses[id] == def.obj {
					uses = append(uses, id)
				}
			}
			return true
		})

		if len(uses) == 0 {
			continue
		}

		// Check if every use is operand of `== nil` which is the entire condition of an if with no else
		allSuccessOnly := true
		for _, use := range uses {
			if !isSuccessOnlyUse(use, parentMap) {
				allSuccessOnly = false
				break
			}
		}

		if allSuccessOnly {
			findings = append(findings, rawFinding{
				pos:      def.pos,
				expr:     def.call,
				category: CategoryErrorOnlyCheckedForSuccess,
			})
		}
	}

	return findings
}

func getVarObj(info *types.Info, id *ast.Ident) (*types.Var, bool) {
	if obj, ok := info.Defs[id]; ok {
		if v, ok := obj.(*types.Var); ok {
			return v, true
		}
	}
	if obj, ok := info.Uses[id]; ok {
		if v, ok := obj.(*types.Var); ok {
			return v, true
		}
	}
	return nil, false
}

func isSuccessOnlyUse(use *ast.Ident, parentMap map[ast.Node]ast.Node) bool {
	parent := parentMap[use]
	if paren, ok := parent.(*ast.ParenExpr); ok {
		parent = parentMap[paren]
	}
	bin, ok := parent.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}

	// Must compare against nil
	if !isNilExpr(bin.X) && !isNilExpr(bin.Y) {
		return false
	}

	grandParent := parentMap[bin]
	if paren, ok := grandParent.(*ast.ParenExpr); ok {
		grandParent = parentMap[paren]
	}

	ifStmt, ok := grandParent.(*ast.IfStmt)
	if !ok {
		return false
	}

	// BinaryExpr must be the entire condition of the if statement
	if ifStmt.Cond != bin {
		return false
	}

	// Must have no else branch
	if ifStmt.Else != nil {
		return false
	}

	return true
}

func isNilExpr(e ast.Expr) bool {
	if id, ok := e.(*ast.Ident); ok && id.Name == "nil" {
		return true
	}
	return false
}

func formatCallee(expr ast.Expr, info *types.Info) string {
	if expr == nil {
		return "<unknown>"
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return types.ExprString(expr)
	}

	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if selection, ok := info.Selections[sel]; ok {
			if fn, ok := selection.Obj().(*types.Func); ok {
				return fn.FullName()
			}
		} else if obj, ok := info.Uses[sel.Sel]; ok {
			if fn, ok := obj.(*types.Func); ok {
				return fn.FullName()
			}
		}
		return types.ExprString(sel)
	}
	if id, ok := call.Fun.(*ast.Ident); ok {
		if obj, ok := info.Uses[id]; ok {
			if fn, ok := obj.(*types.Func); ok {
				return fn.FullName()
			}
		}
		return id.Name
	}
	return types.ExprString(call.Fun)
}
