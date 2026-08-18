package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// isExpandedSetForm reports whether the yaml mapping node m is the expanded
// set-entry form ({ value: ..., secret: true }) rather than a plain map
// literal. True only when m has a "value" key and every key is one of the
// reserved words "value"/"secret" — a map literal that happens to use one of
// those words alongside other keys (e.g. { value: x, count: y }) still reads
// as a map literal.
func isExpandedSetForm(m *yaml.Node) bool {
	hasValue := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		switch m.Content[i].Value {
		case "value":
			hasValue = true
		case "secret":
		default:
			return false
		}
	}
	return hasValue
}

// parseSetNode parses the sequence form shared by set:, with:, and vars:.
func parseSetNode(n *yaml.Node) ([]SetEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("set must be a sequence of key-value maps")
	}
	var entries []SetEntry
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each set entry must be a single key: value pair")
		}
		valNode := item.Content[1]
		if valNode.Kind == yaml.MappingNode && isExpandedSetForm(valNode) {
			// expanded form: { value: ..., secret: true }
			key := item.Content[0].Value
			if strings.Contains(key, "{{") {
				return nil, fmt.Errorf("variable name %q must not contain template syntax", key)
			}
			entry := SetEntry{Key: key}
			for j := 0; j+1 < len(valNode.Content); j += 2 {
				switch valNode.Content[j].Value {
				case "value":
					entry.ValTmpl = normalizeTmpl(valNode.Content[j+1].Value)
				case "secret":
					entry.Secret = parseBool(valNode.Content[j+1])
				}
			}
			entries = append(entries, entry)
			continue
		}
		if valNode.Kind == yaml.MappingNode {
			// map literal: { key: value, ... } -> JSON object template string
			rawKey := item.Content[0].Value
			if strings.Contains(rawKey, "{{") {
				return nil, fmt.Errorf("variable name %q must not contain template syntax", rawKey)
			}
			var raw interface{}
			if err := valNode.Decode(&raw); err != nil {
				return nil, fmt.Errorf("set entry %q: failed to decode map: %w", rawKey, err)
			}
			jsonBytes, err := json.Marshal(raw)
			if err != nil {
				return nil, fmt.Errorf("set entry %q: failed to serialize map: %w", rawKey, err)
			}
			entries = append(entries, SetEntry{Key: rawKey, ValTmpl: string(jsonBytes)})
			continue
		}
		valTmpl := normalizeTmpl(valNode.Value)
		if valNode.Kind == yaml.SequenceNode {
			items := make([]string, len(valNode.Content))
			for i, child := range valNode.Content {
				items[i] = child.Value
			}
			jsonBytes, err := json.Marshal(items)
			if err != nil {
				return nil, fmt.Errorf("set entry %q: failed to serialize list: %w", item.Content[0].Value, err)
			}
			valTmpl = string(jsonBytes)
		}
		rawKey := item.Content[0].Value
		if strings.Contains(rawKey, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", rawKey)
		}
		entries = append(entries, SetEntry{
			Key:     rawKey,
			ValTmpl: valTmpl,
		})
	}
	return entries, nil
}

func parseIntoNode(n *yaml.Node) ([]IntoEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("into must be a sequence of key: value maps")
	}
	var entries []IntoEntry
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each into entry must be a single key: value pair")
		}
		parentKey := item.Content[0].Value
		if strings.Contains(parentKey, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", parentKey)
		}
		entries = append(entries, IntoEntry{
			ParentKey: parentKey,
			ValueTmpl: item.Content[1].Value,
		})
	}
	return entries, nil
}
