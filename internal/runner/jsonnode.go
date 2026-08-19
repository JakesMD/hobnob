package runner

import (
	"encoding/json"
	"fmt"

	"hobnob/internal/config"
)

// evalJSONNode walks a config.JSONNode, calling evalLeaf on every JSONString
// leaf. JSONLiteral values pass through untouched (they were never
// templates); JSONObject/JSONArray recurse into map[string]any/[]any.
func evalJSONNode(n config.JSONNode, evalLeaf func(string) (string, error)) (any, error) {
	switch n.Kind {
	case config.JSONObject:
		obj := make(map[string]any, len(n.Fields))
		for _, field := range n.Fields {
			val, err := evalJSONNode(field.Node, evalLeaf)
			if err != nil {
				return nil, err
			}
			obj[field.Key] = val
		}
		return obj, nil
	case config.JSONArray:
		arr := make([]any, len(n.Elements))
		for i, elem := range n.Elements {
			val, err := evalJSONNode(elem, evalLeaf)
			if err != nil {
				return nil, err
			}
			arr[i] = val
		}
		return arr, nil
	case config.JSONLiteral:
		return n.Literal, nil
	default: // config.JSONString
		return evalLeaf(n.Tmpl)
	}
}

// evalJSONNodeToJSON evaluates n and marshals the fully-assembled result
// exactly once. Marshaling after every leaf is a real Go value — rather than
// substituting evaluated text into an already-serialized JSON string — is
// what lets json.Marshal correctly escape quotes/backslashes/newlines in
// captured data.
func evalJSONNodeToJSON(n config.JSONNode, evalLeaf func(string) (string, error)) (string, error) {
	val, err := evalJSONNode(n, evalLeaf)
	if err != nil {
		return "", err
	}
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("failed to serialize: %w", err)
	}
	return string(jsonBytes), nil
}
