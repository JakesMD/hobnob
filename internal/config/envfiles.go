package config

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"hobnob/internal/eval"

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

// LoadEnvFiles resolves each entry against scope and taskfileDir, then loads
// it: .sh files are sourced in a subshell (see eval.SourceShellFile),
// anything else is parsed as KEY=VALUE lines. Later entries override earlier
// ones in the returned maps.
// Vars default to secret: false; an entry's own secret: true opts it into
// masking.
// A referenced file that doesn't exist prints a warning to stderr and is
// skipped, rather than failing the whole run — a typo'd optional env file
// shouldn't block every task.
func LoadEnvFiles(ctx context.Context, entries []EnvFileEntry, taskfileDir string, scope map[string]string) (values map[string]string, secrets map[string]bool, err error) {
	values = make(map[string]string)
	secrets = make(map[string]bool)
	scopeSoFar := eval.CopyVars(scope)
	for _, entry := range entries {
		path, err := eval.EvalTemplate(entry.PathTmpl, scopeSoFar)
		if err != nil {
			return nil, nil, fmt.Errorf("env file %q: %w", entry.PathTmpl, err)
		}
		path = eval.ResolvePath(path, taskfileDir)

		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "warning: env file %q not found, skipping\n", path)
			continue
		}

		var vars map[string]string
		isShellFile := strings.HasSuffix(path, ".sh")
		isSecret := false
		if entry.SecretOverride != nil {
			isSecret = *entry.SecretOverride
		}
		if isShellFile {
			vars, err = eval.SourceShellFile(ctx, path, taskfileDir)
		} else {
			vars, err = parseDotenvFile(path)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("env file %q: %w", path, err)
		}

		for varName, varValue := range vars {
			values[varName] = varValue
			secrets[varName] = isSecret
			scopeSoFar[varName] = varValue
		}
	}
	return values, secrets, nil
}

func parseDotenvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := eval.SplitKV(line)
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		vars[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}
