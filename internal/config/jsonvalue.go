package config

import "hobnob/internal/value"

// EvalJSONNode walks a JSONNode, calling evalLeaf on every JSONString leaf
// and assembling the result as a real Go tree (json.Number/string/bool/
// []any/map[string]any) wrapped in a Value — never marshaling to JSON text
// and back. JSONLiteral values pass through untouched (they were never
// templates); JSONObject/JSONArray recurse.
//
// evalLeaf's grammar varies by caller: set:/with:/vars: leaves are Go
// templates (eval.EvalValue), run: into: leaves are stdout|filter
// expressions (eval.EvalRunIntoPipe), call: into: leaves are a template or a
// bare .KEY reference into the child scope. EvalJSONNode is agnostic to all
// of that — it only assembles the shape.
func EvalJSONNode(n JSONNode, evalLeaf func(string) (value.Value, error)) (value.Value, error) {
	tree, err := evalJSONNodeTree(n, evalLeaf)
	if err != nil {
		return value.Value{}, err
	}
	return value.Of(tree), nil
}

func evalJSONNodeTree(n JSONNode, evalLeaf func(string) (value.Value, error)) (any, error) {
	switch n.Kind {
	case JSONObject:
		obj := make(map[string]any, len(n.Fields))
		for _, field := range n.Fields {
			val, err := evalJSONNodeTree(field.Node, evalLeaf)
			if err != nil {
				return nil, err
			}
			obj[field.Key] = val
		}
		return obj, nil
	case JSONArray:
		arr := make([]any, len(n.Elements))
		for i, elem := range n.Elements {
			val, err := evalJSONNodeTree(elem, evalLeaf)
			if err != nil {
				return nil, err
			}
			arr[i] = val
		}
		return arr, nil
	case JSONLiteral:
		return n.Literal, nil
	default: // JSONString
		val, err := evalLeaf(n.Tmpl)
		if err != nil {
			return nil, err
		}
		return val.Any(), nil
	}
}

// EvalSetEntry evaluates a SetEntry — shared shape across set:, with:, and
// vars: — against evalLeaf: a map/list literal (ValNode) walks the JSON
// tree, a plain scalar (ValTmpl) evaluates directly.
func EvalSetEntry(entry SetEntry, evalLeaf func(string) (value.Value, error)) (value.Value, error) {
	if entry.ValNode != nil {
		return EvalJSONNode(*entry.ValNode, evalLeaf)
	}
	return evalLeaf(entry.ValTmpl)
}
