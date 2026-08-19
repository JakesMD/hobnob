package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
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
			case "options":
				if fieldVal.Kind == yaml.SequenceNode {
					for _, option := range fieldVal.Content {
						entry.FromList = append(entry.FromList, option.Value)
					}
				} else {
					entry.FromTmpl = normalizeTmpl(fieldVal.Value)
				}
			case "multi":
				entry.Multi = parseBool(fieldVal)
			case "check":
				entry.Check = fieldVal.Value
			case "default":
				entry.DefaultTmpl = normalizeTmpl(fieldVal.Value)
			case "secret":
				entry.Secret = parseBool(fieldVal)
			case "optional":
				entry.Optional = parseBool(fieldVal)
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
