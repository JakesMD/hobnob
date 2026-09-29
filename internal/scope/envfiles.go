package scope

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// loadEnvFiles resolves each entry against scope and taskfileDir, then loads
// it: .sh files are sourced in a subshell (see eval.SourceShellFile),
// anything else is parsed as KEY=VALUE lines. Later entries override earlier
// ones in the returned maps.
// Vars default to secret: false; an entry's own secret: true opts it into
// masking.
// A referenced file that doesn't exist prints a warning to stderr and is
// skipped, rather than failing the whole run — a typo'd optional env file
// shouldn't block every task.
func loadEnvFiles(ctx context.Context, entries []config.EnvFileEntry, taskfileDir string, scope map[string]value.Value) (values map[string]string, secrets map[string]bool, err error) {
	values = make(map[string]string)
	secrets = make(map[string]bool)
	scopeSoFar := eval.CloneMap(scope)
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
			scopeSoFar[varName] = value.Str(varValue)
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
