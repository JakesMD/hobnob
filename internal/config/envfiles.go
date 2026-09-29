package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseEnvNode parses the env: block: a sequence of either bare file paths,
// or the expanded path: { modifiers } form.
func parseEnvNode(node *yaml.Node) ([]EnvFileEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("env must be a sequence of file paths")
	}
	var entries []EnvFileEntry
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			entries = append(entries, EnvFileEntry{PathTmpl: item.Value})
			continue
		}
		// expanded form: - path: { secret: false }
		if item.Kind != yaml.MappingNode || len(item.Content) != 2 {
			return nil, fmt.Errorf("each env entry must be a file path or a single path: modifiers pair")
		}
		entry := EnvFileEntry{PathTmpl: item.Content[0].Value}
		modifiers := item.Content[1]
		if modifiers.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("env entry %q modifiers must be a mapping", entry.PathTmpl)
		}
		for _, modifier := range mapEntries(modifiers.Content) {
			if modifier.Key == "secret" {
				secretOverride := parseBool(modifier.Val)
				entry.SecretOverride = &secretOverride
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
