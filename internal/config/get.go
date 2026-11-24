package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"hobnob/internal/literal"
)

func parseGetNode(node *yaml.Node) ([]GetEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("get must be a sequence")
	}
	var entries []GetEntry
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			if err := validateVarName(item.Value); err != nil {
				return nil, err
			}
			entries = append(entries, GetEntry{VarName: item.Value})
			continue
		}
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each get entry must be a single key: modifiers pair")
		}
		varName := item.Content[0].Value
		if err := validateVarName(varName); err != nil {
			return nil, err
		}
		entry := GetEntry{VarName: varName}
		modifiers := item.Content[1]
		if modifiers.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("get entry %q modifiers must be a mapping", entry.VarName)
		}
		for _, modifier := range mapEntries(modifiers.Content) {
			fieldKey, fieldVal := modifier.Key, modifier.Val
			switch fieldKey {
			case "info":
				entry.Info = fieldVal.Value
			case "type":
				switch fieldVal.Value {
				case GetTypeText, GetTypeFile, GetTypeDir:
					entry.Type = fieldVal.Value
				default:
					return nil, fmt.Errorf("get entry %q: type: must be text, file or dir, got %q", entry.VarName, fieldVal.Value)
				}
			case "options":
				if fieldVal.Kind == yaml.SequenceNode {
					for _, option := range fieldVal.Content {
						entry.FromList = append(entry.FromList, option.Value)
					}
				} else {
					tmpl, err := literal.Single(fieldVal.Value)
					if err != nil {
						return nil, fmt.Errorf("get entry %q options: %w", entry.VarName, err)
					}
					entry.FromTmpl = tmpl
				}
			case "match":
				if fieldVal.Kind == yaml.SequenceNode {
					for _, pattern := range fieldVal.Content {
						entry.MatchList = append(entry.MatchList, pattern.Value)
					}
				} else {
					tmpl, err := literal.Single(fieldVal.Value)
					if err != nil {
						return nil, fmt.Errorf("get entry %q match: %w", entry.VarName, err)
					}
					entry.MatchTmpl = tmpl
				}
			case "multi":
				entry.Multi = parseBool(fieldVal)
			case "check":
				entry.Check = fieldVal.Value
			case "default":
				tmpl, err := literal.Single(fieldVal.Value)
				if err != nil {
					return nil, fmt.Errorf("get entry %q default: %w", entry.VarName, err)
				}
				entry.DefaultTmpl = tmpl
			case "secret":
				entry.Secret = parseBool(fieldVal)
			case "optional":
				entry.Optional = parseBool(fieldVal)
			}
		}
		if entry.IsPath() && (len(entry.FromList) > 0 || !entry.FromTmpl.IsZero()) {
			return nil, fmt.Errorf("get entry %q: options: can't be used with type: %s; leave out type: for a menu of paths", entry.VarName, entry.Type)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
