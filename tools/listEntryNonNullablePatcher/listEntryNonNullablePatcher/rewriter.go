package listEntryNonNullablePatcher

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

var Analyzer = &analysis.Analyzer{
	Name:             "listEntryNonNullablePatcher",
	Doc:              "Add ListEntryNonNullable schemabuilder option to fieldfuncs that returns a list with pointer entries",
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	Run:              run,
	RunDespiteErrors: true,
}

// fileLine encodes a location as a combination of a file name and a line number.
type fileLine struct {
	FileName   string
	LineNumber int
}

func rewrite(pass *analysis.Pass, callExpr *ast.CallExpr) {
	newText := ",schemabuilder.ListEntryNonNullable"
	// If the last ")" is not at the same line as the last argument, we don't need the "," prefix.
	if pass.Fset.Position(callExpr.Rparen).Line != pass.Fset.Position(callExpr.Args[len(callExpr.Args)-1].End()).Line {
		newText = "schemabuilder.ListEntryNonNullable"
	}
	pass.Report(analysis.Diagnostic{
		Pos:     callExpr.Rparen,
		Message: "Fix",
		SuggestedFixes: []analysis.SuggestedFix{
			{
				TextEdits: []analysis.TextEdit{
					{
						Pos:     callExpr.Rparen,
						End:     callExpr.Rparen,
						NewText: []byte(newText),
					},
				},
			},
		},
	})
}

func unwrapNamed(typ types.Type) types.Type {
	if named, isNamed := typ.(*types.Named); isNamed {
		return unwrapNamed(named.Underlying())
	}
	return typ
}

func unwrapIdent(typ ast.Expr) ast.Expr {
	if ident, ok := typ.(*ast.Ident); ok {
		// If the response type is a custom type.
		if ident.Obj != nil {
			if typeSpec, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
				return unwrapIdent(typeSpec.Type)
			}
		}
	}
	return typ
}

func run(pass *analysis.Pass) (interface{}, error) {
	fmt.Println("hello")
	callFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	inspect.Nodes(callFilter, func(node ast.Node, push bool) bool {
		if !push {
			return true
		}

		callExpr, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		invocation, ok := typeutil.Callee(pass.TypesInfo, callExpr).(*types.Func)
		if !ok {
			return true
		}

		signature, ok := invocation.Type().(*types.Signature)
		if !ok {
			return true
		}
		receiver := signature.Recv()
		if receiver == nil {
			return true
		}
		receiverType := receiver.Type().String()

		// Check if it's a FieldFunc.
		if !strings.HasSuffix(receiverType, ".Object") || len(callExpr.Args) <= 1 {
			return true
		}

		if invocation.Name() != "FieldFunc" &&
			invocation.Name() != "BatchFieldFunc" &&
			invocation.Name() != "BatchFieldFuncWithFallback" &&
			invocation.Name() != "ManualPaginationWithFallback" {
			return true
		}

		for _, arg := range callExpr.Args {
			if sel, ok := arg.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "ListEntryNonNullable" {
					return true
				}
			}
		}

		// If the fieldfunc is a method of a struct.
		if fieldFunc, ok := callExpr.Args[1].(*ast.SelectorExpr); ok {
			if sel, ok := pass.TypesInfo.Selections[fieldFunc]; ok {
				if sig, ok := sel.Type().(*types.Signature); ok {
					if sig.Results().Len() > 0 {
						resultType := unwrapNamed(sig.Results().At(0).Type())
						// BatchFieldFunc: map[index][]*object.
						if mapType, ok := resultType.(*types.Map); ok {
							if slice, ok := unwrapNamed(mapType.Elem()).(*types.Slice); ok {
								if _, isPtr := unwrapNamed(slice.Elem()).(*types.Pointer); isPtr {
									rewrite(pass, callExpr)
								}
							}
						}
						// Fieldfunc: []*object.
						if slice, ok := resultType.(*types.Slice); ok {
							elemType := unwrapNamed(slice.Elem())
							if _, isPtr := elemType.(*types.Pointer); isPtr {
								rewrite(pass, callExpr)
							}
						}
					}
				}
			}
			return true
		}

		var funcType *ast.FuncType
		if fieldFunc, ok := callExpr.Args[1].(*ast.FuncLit); ok {
			// If the fieldfunc is an anonymous function.
			funcType = fieldFunc.Type
		} else if fieldFunc, ok := callExpr.Args[1].(*ast.Ident); ok {
			// If the fieldfunc is a normal function.
			if fieldFunc.Obj != nil && fieldFunc.Obj.Decl != nil {
				if funcDecl, ok := fieldFunc.Obj.Decl.(*ast.FuncDecl); ok {
					funcType = funcDecl.Type
				}
			}
		}

		if funcType == nil {
			return true
		}

		if len(funcType.Results.List) > 0 {
			resultType := unwrapIdent(funcType.Results.List[0].Type)

			var sliceType *ast.ArrayType
			var isSlice bool

			if mapType, ok := resultType.(*ast.MapType); ok {
				// BatchFieldFunc: map[index][]*object.
				sliceType, isSlice = unwrapIdent(mapType.Value).(*ast.ArrayType)
			} else {
				// FieldFunc: []*object.
				sliceType, isSlice = resultType.(*ast.ArrayType)
			}

			if isSlice {
				eltType := unwrapIdent(sliceType.Elt)
				if _, isPtr := eltType.(*ast.StarExpr); isPtr {
					rewrite(pass, callExpr)
				}
			}
		}

		return true
	})

	return nil, nil
}
