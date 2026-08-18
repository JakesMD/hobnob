package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseGetNode(n *yaml.Node) ([]GetEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("get must be a sequence")
	}
	var entries []GetEntry
	for _, item := range n.Content {
		if item.Kind == yaml.ScalarNode {
			if strings.Contains(item.Value, "{{") {
				return nil, fmt.Errorf("variable name %q must not contain template syntax", item.Value)
			}
			entries = append(entries, GetEntry{VarName: item.Value})
			continue
		}
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each get entry must be a single key: modifiers pair")
		}
		varName := item.Content[0].Value
		if strings.Contains(varName, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", varName)
		}
		e := GetEntry{VarName: varName}
		modifiers := item.Content[1]
		if modifiers.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("get entry %q modifiers must be a mapping", e.VarName)
		}
		for i := 0; i+1 < len(modifiers.Content); i += 2 {
			fieldKey := modifiers.Content[i].Value
			fieldVal := modifiers.Content[i+1]
			switch fieldKey {
			case "info":
				e.Info = fieldVal.Value
			case "options":
				if fieldVal.Kind == yaml.SequenceNode {
					for _, fi := range fieldVal.Content {
						e.FromList = append(e.FromList, fi.Value)
					}
				} else {
					e.FromTmpl = normalizeTmpl(fieldVal.Value)
				}
			case "multi":
				e.Multi = parseBool(fieldVal)
			case "check":
				e.Check = fieldVal.Value
			case "default":
				e.DefaultTmpl = normalizeTmpl(fieldVal.Value)
			case "secret":
				e.Secret = parseBool(fieldVal)
			case "optional":
				e.Optional = parseBool(fieldVal)
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}
