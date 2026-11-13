package config

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
	"hobnob/internal/literal"
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

// errSpreadEntry is the entry-level wording for a spread written as a whole
// set:/into: entry, where literal.ErrSpread would only say it isn't in a
// list.
func errSpreadEntry(raw string) error {
	return fmt.Errorf("a spread is not a KEY: value pair; spread inside a list value instead (KEY: [%s])", strings.TrimSpace(raw))
}

// parseSetNode parses the sequence form shared by set:, with:, and vars:.
func parseSetNode(node *yaml.Node) ([]SetEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("set must be a sequence of key-value maps")
	}
	var entries []SetEntry
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			if _, err := literal.Single(item.Value); errors.Is(err, literal.ErrSpread) {
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
					val, err := literal.Single(modifier.Val.Value)
					if err != nil {
						return nil, fmt.Errorf("set entry %q: %w", key, err)
					}
					entry.Val = val
				case "secret":
					entry.Secret = parseBool(modifier.Val)
				}
			}
			entries = append(entries, entry)
			continue
		}
		rawKey := item.Content[0].Value
		if err := validateVarName(rawKey); err != nil {
			return nil, err
		}
		val, err := literal.Parse(rawKey, valNode)
		if err != nil {
			return nil, fmt.Errorf("set entry %q: %w", rawKey, err)
		}
		entries = append(entries, SetEntry{Key: rawKey, Val: val})
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
			if _, err := literal.ParseInto(name, item); errors.Is(err, literal.ErrSpread) {
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
		val, err := literal.ParseInto(parentKey, item.Content[1])
		if err != nil {
			return nil, fmt.Errorf("into entry %q: %w", parentKey, err)
		}
		entries = append(entries, IntoEntry{ParentKey: parentKey, Val: val})
	}
	return entries, nil
}
