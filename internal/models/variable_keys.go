package models

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/types"
)

const ConfigHelper = "config"

func (r *FrameworkArrayResolver) VariableKeys(res *resolve.Resolver, file *parser.FileNode, v *ast.Variable) []types.ShapeField {
	if keys := shapeFields(res.NodeType(file, v)); len(keys) > 0 {
		return keys
	}
	if value, ok := res.AssignedValue(file, v); ok {
		if keys := r.assignedKeys(res, file, value); len(keys) > 0 {
			return keys
		}
	}
	return offsetWrites(file, v)
}

func shapeFields(rt resolve.ResolvedType) []types.ShapeField {
	fields := make([]types.ShapeField, 0, len(rt.Fields))
	for _, f := range rt.Fields {
		fields = append(fields, types.ShapeField{Key: f.Key, Type: f.Type.FullString(), Optional: f.Optional})
	}
	return fields
}

func (r *FrameworkArrayResolver) assignedKeys(res *resolve.Resolver, file *parser.FileNode, value ast.Expr) []types.ShapeField {
	if r == nil {
		return nil
	}
	src := file.Source
	switch call := value.(type) {
	case *ast.Call:
		if r.framework != "laravel" || sourcectx.FunctionName(src, call) != ConfigHelper {
			return nil
		}
		if len(call.Args.Args) == 0 {
			return r.ConfigKeys("")
		}
		if path, ok := sourcectx.StringValue(src, call.Args.Args[0].Value); ok && path != "" {
			return r.ConfigKeys(path)
		}
	case *ast.MethodCall:
		return r.ResolveMethodReturnKeys(res.NodeType(file, call.Object).BaseFQN(), sourcectx.MethodName(src, call))
	}
	return nil
}

func offsetWrites(file *parser.FileNode, v *ast.Variable) []types.ShapeField {
	src := file.Source
	name := v.Bounds().Text(src)
	body := enclosingFunction(file.Tree, v.Start)
	var keys []types.ShapeField
	seen := make(map[string]bool)
	ast.Walk(body, func(n ast.Node) bool {
		if n != body && isFunctionLike(n) {
			return false
		}
		assign, ok := n.(*ast.Assign)
		if !ok || assign.Op != token.Equals {
			return true
		}
		dim, ok := assign.Target.(*ast.ArrayDimFetch)
		if !ok {
			return true
		}
		if target, ok := dim.Array.(*ast.Variable); !ok || target.Bounds().Text(src) != name {
			return true
		}
		if key, ok := sourcectx.StringValue(src, dim.Dim); ok && !seen[key] {
			seen[key] = true
			keys = append(keys, types.ShapeField{Key: key})
		}
		return true
	})
	return keys
}

func enclosingFunction(root ast.Node, off uint32) ast.Node {
	found := root
	ast.Walk(root, func(n ast.Node) bool {
		span := n.Bounds()
		if off < span.Start || off > span.End {
			return false
		}
		if isFunctionLike(n) {
			found = n
		}
		return true
	})
	return found
}

func isFunctionLike(n ast.Node) bool {
	switch n.(type) {
	case *ast.FunctionDecl, *ast.Method, *ast.Closure, *ast.ArrowFunction:
		return true
	}
	return false
}
