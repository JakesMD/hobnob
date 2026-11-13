package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
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

// leafGrammar turns a literal's raw string leaf into the text stored for it,
// and returns the reference when the leaf is a spread (REF...).
type leafGrammar func(raw string) (tmpl, spread string)

// templateLeaf is the leaf grammar of set:/with:/const:/vars: literals and
// run: argv elements, where bare .VAR shorthand works as in any other field.
func templateLeaf(raw string) (string, string) {
	if ref, ok := eval.CutSpread(raw); ok {
		return eval.WrapRef(ref), ref
	}
	if eval.IsBareRef(raw) {
		return eval.WrapRef(raw), ""
	}
	return raw, ""
}

// intoLeaf is the leaf grammar of into: literals: its own stdout|filter /
// .FIELD grammar, never Go templates, so the text is stored as written.
func intoLeaf(raw string) (string, string) {
	if ref, ok := eval.CutIntoSpread(raw); ok {
		return ref, ref
	}
	return raw, ""
}

func singleValue(grammar leafGrammar, raw string) (string, error) {
	tmpl, spread := grammar(raw)
	if spread != "" {
		return "", errSpreadOutsideList(raw)
	}
	return tmpl, nil
}

func errSpreadOutsideList(raw string) error {
	return fmt.Errorf("%q spreads a list, so it only works as a list element; drop the ... or wrap it in [ ]", strings.TrimSpace(raw))
}

func errSpreadEntry(raw string) error {
	return fmt.Errorf("a spread is not a KEY: value pair; spread inside a list value instead (KEY: [%s])", strings.TrimSpace(raw))
}

// parseJSONLiteralNode recursively parses a YAML mapping/sequence/scalar into
// a JSONNode tree, storing each string leaf as grammar returns it.
//
// path is the dotted field path to n, used only to prefix error messages so
// a failure several levels deep (e.g. CUSTOM.profile.name) doesn't just
// report the top-level key.
func parseJSONLiteralNode(n *yaml.Node, grammar leafGrammar, path string) (JSONNode, error) {
	if n.Kind == yaml.AliasNode {
		return parseJSONLiteralNode(n.Alias, grammar, path)
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
			child, err := parseJSONLiteralNode(entry.Val, grammar, childPath)
			if err != nil {
				return JSONNode{}, err
			}
			if child.Spread != "" {
				return JSONNode{}, fmt.Errorf("%s: %w", childPath, errSpreadOutsideList(entry.Val.Value))
			}
			fields = append(fields, JSONField{Key: entry.Key, Node: child})
		}
		return JSONNode{Kind: JSONObject, Fields: fields}, nil
	case yaml.SequenceNode:
		elements := make([]JSONNode, len(n.Content))
		for i, item := range n.Content {
			child, err := parseJSONLiteralNode(item, grammar, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return JSONNode{}, err
			}
			elements[i] = child
		}
		return JSONNode{Kind: JSONArray, Elements: elements}, nil
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			tmpl, spread := grammar(n.Value)
			return JSONNode{Kind: JSONString, Tmpl: tmpl, Spread: spread}, nil
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
		if item.Kind == yaml.ScalarNode {
			if _, spread := templateLeaf(item.Value); spread != "" {
				return nil, fmt.Errorf("set entry %q: %w", item.Value, errSpreadEntry(item.Value))
			}
		}
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
					tmpl, err := normalizeTmpl(modifier.Val.Value)
					if err != nil {
						return nil, fmt.Errorf("set entry %q: %w", key, err)
					}
					entry.ValTmpl = tmpl
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
			node, err := parseJSONLiteralNode(valNode, templateLeaf, rawKey)
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
		tmpl, err := normalizeTmpl(valNode.Value)
		if err != nil {
			return nil, fmt.Errorf("set entry %q: %w", rawKey, err)
		}
		entries = append(entries, SetEntry{Key: rawKey, ValTmpl: tmpl})
	}
	return entries, nil
}

func parseIntoNode(node *yaml.Node) ([]IntoEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("into must be a sequence of NAME or NAME: value entries")
	}
	var entries []IntoEntry
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			name := item.Value
			if _, spread := intoLeaf(name); spread != "" {
				return nil, fmt.Errorf("into entry %q: %w", name, errSpreadEntry(name))
			}
			if !eval.IsVarName(name) {
				return nil, fmt.Errorf("into entry %q: a bare entry must be a variable name", name)
			}
			entries = append(entries, BareIntoEntry(name))
			continue
		}
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each into entry must be a NAME or a single NAME: value pair")
		}
		parentKey := item.Content[0].Value
		if err := validateVarName(parentKey); err != nil {
			return nil, err
		}
		valNode := item.Content[1]
		if valNode.Kind == yaml.MappingNode || valNode.Kind == yaml.SequenceNode {
			node, err := parseJSONLiteralNode(valNode, intoLeaf, parentKey)
			if err != nil {
				return nil, fmt.Errorf("into entry %q: %w", parentKey, err)
			}
			entries = append(entries, IntoEntry{ParentKey: parentKey, ValNode: &node})
			continue
		}
		leaf, err := singleValue(intoLeaf, valNode.Value)
		if err != nil {
			return nil, fmt.Errorf("into entry %q: %w", parentKey, err)
		}
		entries = append(entries, IntoEntry{ParentKey: parentKey, ValueTmpl: leaf})
	}
	return entries, nil
}
