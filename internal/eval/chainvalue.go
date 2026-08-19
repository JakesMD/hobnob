package eval

import (
	"encoding/json"
	"text/template/parse"

	"hobnob/internal/value"
)

// EvalValue evaluates expr against vars, preserving structure when expr is
// exactly one template action referencing a var, optionally through a
// filter chain — ".USERS", "{{ .USERS }}", "{{ .USERS | pluck \"name\" }}".
// Anything else (surrounding text, multiple actions, control structures, a
// shape the typed evaluator below doesn't recognize) falls back to ordinary
// string rendering and comes back wrapped in a String — unsupported syntax
// still works, it just loses its type.
func EvalValue(expr string, vars map[string]value.Value) (value.Value, error) {
	tmpl, err := parseTemplateCached(expr)
	if err != nil {
		return value.Value{}, err
	}
	if pipe, ok := singleActionPipeline(tmpl.Tree.Root); ok {
		if result, matched, err := evalTypedPipeline(pipe, vars); matched {
			return result, err
		}
	}
	rendered, err := EvalTemplate(expr, vars)
	if err != nil {
		return value.Value{}, err
	}
	return value.Str(rendered), nil
}

// evalChainOn evaluates a filter chain — the text written after "| " in a
// run: into: pipe expression, e.g. `trim` or `pluck "id"` — against a single
// starting value, through the same typed pipeline evaluator EvalValue uses.
// One implementation backs both run: into: pipes and set:/get: filter
// chains.
func evalChainOn(src value.Value, chain string) (value.Value, error) {
	return EvalValue("{{ .SRC | "+chain+" }}", map[string]value.Value{"SRC": src})
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

// evalTypedPipeline evaluates pipe's commands without ever rendering to
// text: the first command resolves a var reference or a literal, each later
// command applies a value.Filters entry to the running value.
//
// matched is false when the pipeline uses a shape this function doesn't
// recognize — a literal-only source with no filters wouldn't need typed
// evaluation anyway, a multi-arg function call as the source, a nested
// sub-pipeline argument, an unregistered function name. Callers fall back
// to string rendering rather than erroring, so unrecognized syntax still
// works, just untyped. err is only meaningful when matched is true.
func evalTypedPipeline(pipe *parse.PipeNode, vars map[string]value.Value) (result value.Value, matched bool, err error) {
	if len(pipe.Cmds) == 0 {
		return value.Value{}, false, nil
	}
	current, ok := evalSource(pipe.Cmds[0], vars)
	if !ok {
		return value.Value{}, false, nil
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

// evalSource evaluates a pipeline's first command: a bare var reference or a
// literal. Any other shape (a function call, a nested pipeline, a
// multi-segment field like .VAR.Field — scope values are never nested that
// deep) is unrecognized.
func evalSource(cmd *parse.CommandNode, vars map[string]value.Value) (value.Value, bool) {
	if len(cmd.Args) != 1 {
		return value.Value{}, false
	}
	return literalOrField(cmd.Args[0], vars)
}

func literalOrField(node parse.Node, vars map[string]value.Value) (value.Value, bool) {
	switch n := node.(type) {
	case *parse.FieldNode:
		if len(n.Ident) != 1 {
			return value.Value{}, false
		}
		return vars[n.Ident[0]], true
	case *parse.StringNode:
		return value.Str(n.Text), true
	case *parse.NumberNode:
		return value.Of(json.Number(n.Text)), true
	case *parse.BoolNode:
		return value.Of(n.True), true
	default:
		return value.Value{}, false
	}
}

// evalFilterCommand applies one pipeline command — IDENT arg1 arg2 ... — as
// a value.Filters lookup, with piped appended as the trailing argument
// (matching Go template's own pipe convention).
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
		v, ok := literalOrField(argNode, vars)
		if !ok {
			return value.Value{}, false, nil
		}
		args = append(args, v)
	}
	args = append(args, piped)
	result, err := filter(args)
	return result, true, err
}
