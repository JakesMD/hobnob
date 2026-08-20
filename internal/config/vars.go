package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"hobnob/internal/value"
)

// isExpandedSetForm reports whether the yaml mapping node m is the expanded
// set-entry form ({ value: ..., secret: true }) rather than a plain map
// literal. True only when m has a "value" key and every key is one of the
// reserved words "value"/"secret" — a map literal that happens to use one of
// those words alongside other keys (e.g. { value: x, count: y }) still reads
// as a map literal.
func isExpandedSetForm(mapping *yaml.Node) bool {
	hasValue := false
	for _, entry := range mapEntries(mapping.Content) {
		switch entry.Key {
		case "value":
			hasValue = true
		case "secret":
		default:
			return false
		}
	}
	return hasValue
}

// parseJSONLiteralNode recursively parses a YAML mapping/sequence/scalar into
// a JSONNode tree. transformLeaf is applied to every string-scalar leaf's raw
// text before it's stored — set:/with: pass normalizeTmpl (so bare .VAR
// shorthand keeps working inside map/list literals the same as it does for a
// plain scalar entry); into: passes an identity function, since its leaves
// are its own stdout|filter / .FIELD grammar, never Go templates.
//
// path is the dotted field path to n, used only to prefix error messages so
// a failure several levels deep (e.g. CUSTOM.profile.name) doesn't just
// report the top-level key.
func parseJSONLiteralNode(n *yaml.Node, transformLeaf func(string) string, path string) (JSONNode, error) {
	if n.Kind == yaml.AliasNode {
		return parseJSONLiteralNode(n.Alias, transformLeaf, path)
	}
	switch n.Kind {
	case yaml.MappingNode:
		fields := make([]JSONField, 0, len(n.Content)/2)
		for _, entry := range mapEntries(n.Content) {
			childPath := entry.Key
			if path != "" {
				childPath = path + "." + entry.Key
			}
			if err := validateVarName(entry.Key); err != nil {
				return JSONNode{}, fmt.Errorf("%s: %w", childPath, err)
			}
			child, err := parseJSONLiteralNode(entry.Val, transformLeaf, childPath)
			if err != nil {
				return JSONNode{}, err
			}
			fields = append(fields, JSONField{Key: entry.Key, Node: child})
		}
		return JSONNode{Kind: JSONObject, Fields: fields}, nil
	case yaml.SequenceNode:
		elements := make([]JSONNode, len(n.Content))
		for i, item := range n.Content {
			child, err := parseJSONLiteralNode(item, transformLeaf, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return JSONNode{}, err
			}
			elements[i] = child
		}
		return JSONNode{Kind: JSONArray, Elements: elements}, nil
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			return JSONNode{Kind: JSONString, Tmpl: transformLeaf(n.Value)}, nil
		}
		var literal any
		if err := n.Decode(&literal); err != nil {
			return JSONNode{}, fmt.Errorf("%s: failed to decode literal %q: %w", path, n.Value, err)
		}
		return JSONNode{Kind: JSONLiteral, Literal: value.Canonical(literal)}, nil
	default:
		return JSONNode{}, fmt.Errorf("%s: unsupported YAML node in JSON literal", path)
	}
}

// parseSetNode parses the sequence form shared by set:, with:, and vars:.
func parseSetNode(node *yaml.Node) ([]SetEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("set must be a sequence of key-value maps")
	}
	var entries []SetEntry
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each set entry must be a single key: value pair")
		}
		valNode := item.Content[1]
		if valNode.Kind == yaml.MappingNode && isExpandedSetForm(valNode) {
			// expanded form: { value: ..., secret: true }
			key := item.Content[0].Value
			if err := validateVarName(key); err != nil {
				return nil, err
			}
			entry := SetEntry{Key: key}
			for _, modifier := range mapEntries(valNode.Content) {
				switch modifier.Key {
				case "value":
					entry.ValTmpl = normalizeTmpl(modifier.Val.Value)
				case "secret":
					entry.Secret = parseBool(modifier.Val)
				}
			}
			entries = append(entries, entry)
			continue
		}
		if valNode.Kind == yaml.MappingNode || valNode.Kind == yaml.SequenceNode {
			// map/list literal -> deferred JSON tree, evaluated and
			// marshaled once at runtime (see JSONNode).
			rawKey := item.Content[0].Value
			if err := validateVarName(rawKey); err != nil {
				return nil, err
			}
			node, err := parseJSONLiteralNode(valNode, normalizeTmpl, rawKey)
			if err != nil {
				return nil, fmt.Errorf("set entry %q: %w", rawKey, err)
			}
			entries = append(entries, SetEntry{Key: rawKey, ValNode: &node})
			continue
		}
		rawKey := item.Content[0].Value
		if err := validateVarName(rawKey); err != nil {
			return nil, err
		}
		entries = append(entries, SetEntry{
			Key:     rawKey,
			ValTmpl: normalizeTmpl(valNode.Value),
		})
	}
	return entries, nil
}

func parseIntoNode(node *yaml.Node) ([]IntoEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("into must be a sequence of key: value maps")
	}
	var entries []IntoEntry
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each into entry must be a single key: value pair")
		}
		parentKey := item.Content[0].Value
		if err := validateVarName(parentKey); err != nil {
			return nil, err
		}
		valNode := item.Content[1]
		if valNode.Kind == yaml.MappingNode || valNode.Kind == yaml.SequenceNode {
			node, err := parseJSONLiteralNode(valNode, func(s string) string { return s }, parentKey)
			if err != nil {
				return nil, fmt.Errorf("into entry %q: %w", parentKey, err)
			}
			entries = append(entries, IntoEntry{ParentKey: parentKey, ValNode: &node})
			continue
		}
		entries = append(entries, IntoEntry{
			ParentKey: parentKey,
			ValueTmpl: valNode.Value,
		})
	}
	return entries, nil
}
