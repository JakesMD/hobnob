package eval

import (
	"encoding/json"
	"text/template/parse"

	"hobnob/internal/value"
)

// EvalValue evaluates expr against vars, preserving structure when expr is
// exactly one template action referencing a var, optionally through a
// filter chain or an accessor — ".USERS", "{{ .USERS }}",
// "{{ .USERS | trim }}", "{{ .RESP.profile.name }}". Anything else
// (surrounding text, multiple actions, control structures, a shape the
// typed evaluator below doesn't recognize) falls back to ordinary string
// rendering and comes back wrapped in a String — unsupported syntax still
// works, it just loses its type.
func EvalValue(expr string, vars map[string]value.Value) (value.Value, error) {
	tmpl, err := parseTemplateCached(expr)
	if err != nil {
		return value.Value{}, err
	}
	if pipe, ok := singleActionPipeline(tmpl.Tree.Root); ok {
		if result, matched, err := evalTypedPipeline(pipe, vars); matched {
			if err != nil {
				return value.Value{}, err
			}
			if result.IsMissing() {
				return value.Value{}, result.MissingErr()
			}
			return result, nil
		}
	}
	rendered, err := EvalTemplate(expr, vars)
	if err != nil {
		return value.Value{}, err
	}
	return value.Str(rendered), nil
}

// evalChainOn evaluates an accessor and/or filter chain — the text written
// after "| " in a run: into: pipe expression, e.g. `trim` or `upper`, plus
// an optional accessor tail peeled off the source token (SplitSourceAccessor)
// — against a single starting value, through the same typed pipeline
// evaluator EvalValue uses. One implementation backs both run: into: pipes
// and set:/get: filter chains. vars is the caller's scope, layered under the
// SRC binding — needed so a dynamic key in the accessor (stdout[.KEY]) can
// resolve against the caller's vars rather than only ever seeing SRC.
func evalChainOn(src value.Value, accessor, chain string, vars map[string]value.Value) (value.Value, error) {
	scoped := make(map[string]value.Value, len(vars)+1)
	for k, v := range vars {
		scoped[k] = v
	}
	scoped["SRC"] = src
	expr := "{{ .SRC" + accessor
	if chain != "" {
		expr += " | " + chain
	}
	expr += " }}"
	return EvalValue(expr, scoped)
}

// singleActionPipeline returns root's pipeline when the whole template is
// exactly one action with no variable declarations ({{ $x := ... }}) — the
// shape a field's value must have for its type to survive evaluation.
func singleActionPipeline(root *parse.ListNode) (*parse.PipeNode, bool) {
	if root == nil || len(root.Nodes) != 1 {
		return nil, false
	}
	action, ok := root.Nodes[0].(*parse.ActionNode)
	if !ok || len(action.Pipe.Decl) != 0 {
		return nil, false
	}
	return action.Pipe, true
}

// pathFuncs maps the identifiers the accessor rewriter (accessor.go) emits
// to their typed implementations — one body serves both text/template's
// FuncMap (registered in template.go, which stringifies via reflection) and
// this typed evaluator.
var pathFuncs = map[string]func([]value.Value) (value.Value, error){
	"hbpath":  value.PathCall,
	"hbslice": value.SliceCall,
	"hbstar":  value.StarCall,
}

// evalTypedPipeline evaluates pipe's commands without ever rendering to
// text: the first command resolves a var reference, a literal, or a
// rewritten accessor call, each later command applies a value.Filters entry
// to the running value.
//
// matched is false when the pipeline uses a shape this function doesn't
// recognize — a literal-only source with no filters wouldn't need typed
// evaluation anyway, a multi-arg function call as the source that isn't a
// path builtin, an unregistered filter name. Callers fall back to string
// rendering rather than erroring, so unrecognized syntax still works, just
// untyped. err is only meaningful when matched is true.
func evalTypedPipeline(pipe *parse.PipeNode, vars map[string]value.Value) (result value.Value, matched bool, err error) {
	if len(pipe.Cmds) == 0 {
		return value.Value{}, false, nil
	}
	current, ok, srcErr := evalSource(pipe.Cmds[0], vars)
	if !ok {
		return value.Value{}, false, nil
	}
	if srcErr != nil {
		return value.Value{}, true, srcErr
	}
	for _, cmd := range pipe.Cmds[1:] {
		next, ok, cmdErr := evalFilterCommand(cmd, vars, current)
		if !ok {
			return value.Value{}, false, nil
		}
		if cmdErr != nil {
			return value.Value{}, true, cmdErr
		}
		current = next
	}
	return current, true, nil
}

// evalSource evaluates a pipeline's first command: a bare var reference, a
// literal, or a rewritten accessor call — (hbpath ...), hbstar, or
// (hbslice ...) nested as a source, e.g. from an accessor used as another
// accessor's dynamic-key subscript. Any other shape (an ordinary function
// call, an unregistered identifier) is unrecognized.
func evalSource(cmd *parse.CommandNode, vars map[string]value.Value) (value.Value, bool, error) {
	if len(cmd.Args) == 1 {
		return literalOrField(cmd.Args[0], vars)
	}
	if ident, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
		if _, known := pathFuncs[ident.Ident]; known {
			return evalPathCommand(cmd, vars)
		}
	}
	return value.Value{}, false, nil
}

func literalOrField(node parse.Node, vars map[string]value.Value) (value.Value, bool, error) {
	switch n := node.(type) {
	case *parse.FieldNode:
		if len(n.Ident) != 1 {
			return value.Value{}, false, nil
		}
		return vars[n.Ident[0]], true, nil
	case *parse.StringNode:
		return value.Str(n.Text), true, nil
	case *parse.NumberNode:
		return value.Of(json.Number(n.Text)), true, nil
	case *parse.BoolNode:
		return value.Of(n.True), true, nil
	case *parse.PipeNode:
		// A parenthesized sub-expression used as an argument — the shape a
		// rewritten (hbpath ...) call takes when nested inside another
		// term's argument list, or a paren-head accessor's own root
		// ((.TAGS | json)[0]).
		return evalTypedPipeline(n, vars)
	case *parse.IdentifierNode:
		if fn, ok := pathFuncs[n.Ident]; ok {
			result, err := fn(nil)
			return result, true, err
		}
		return value.Value{}, false, nil
	default:
		return value.Value{}, false, nil
	}
}

// evalPathCommand evaluates a rewritten accessor call — hbpath/hbslice —
// whose args (after the identifier) are resolved the same way any other
// command's args are: literals, var references, or nested accessor/
// sub-pipeline expressions.
func evalPathCommand(cmd *parse.CommandNode, vars map[string]value.Value) (value.Value, bool, error) {
	ident := cmd.Args[0].(*parse.IdentifierNode)
	fn, ok := pathFuncs[ident.Ident]
	if !ok {
		return value.Value{}, false, nil
	}
	args := make([]value.Value, 0, len(cmd.Args)-1)
	for _, argNode := range cmd.Args[1:] {
		v, ok, err := literalOrField(argNode, vars)
		if err != nil {
			return value.Value{}, true, err
		}
		if !ok {
			return value.Value{}, false, nil
		}
		args = append(args, v)
	}
	result, err := fn(args)
	return result, true, err
}

// evalFilterCommand applies one pipeline command — IDENT arg1 arg2 ... — as
// a value.Filters lookup, with piped appended as the trailing argument
// (matching Go template's own pipe convention). Guards the same
// missing-sentinel exemption adaptFilter enforces on the string-rendering
// path: every filter but default raises a deferred "path not found" value
// as a real error before its own body runs.
func evalFilterCommand(cmd *parse.CommandNode, vars map[string]value.Value, piped value.Value) (value.Value, bool, error) {
	if len(cmd.Args) == 0 {
		return value.Value{}, false, nil
	}
	ident, ok := cmd.Args[0].(*parse.IdentifierNode)
	if !ok {
		return value.Value{}, false, nil
	}
	filter, ok := value.Filters[ident.Ident]
	if !ok {
		return value.Value{}, false, nil
	}
	args := make([]value.Value, 0, len(cmd.Args))
	for _, argNode := range cmd.Args[1:] {
		v, ok, err := literalOrField(argNode, vars)
		if err != nil {
			return value.Value{}, true, err
		}
		if !ok {
			return value.Value{}, false, nil
		}
		args = append(args, v)
	}
	args = append(args, piped)
	if ident.Ident != "default" {
		for _, a := range args {
			if a.IsMissing() {
				return value.Value{}, true, a.MissingErr()
			}
		}
	}
	result, err := filter(args)
	return result, true, err
}
