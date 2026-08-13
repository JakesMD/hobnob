package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/eval"
)

// LoadEnvFiles resolves each entry against scope and taskfileDir, then loads
// it: .sh files are sourced in a subshell (see eval.SourceShellFile),
// anything else is parsed as KEY=VALUE lines. Later entries override earlier
// ones in the returned maps.
// Vars default to secret: false; an entry's own secret: true opts it into
// masking.
// A referenced file that doesn't exist prints a warning to stderr and is
// skipped, rather than failing the whole run — a typo'd optional env file
// shouldn't block every task.
func LoadEnvFiles(entries []EnvFileEntry, taskfileDir string, scope map[string]string) (values map[string]string, secrets map[string]bool, err error) {
	values = make(map[string]string)
	secrets = make(map[string]bool)
	scopeSoFar := eval.CopyVars(scope)
	for _, entry := range entries {
		path, err := eval.EvalTemplate(entry.PathTmpl, scopeSoFar)
		if err != nil {
			return nil, nil, fmt.Errorf("env file %q: %w", entry.PathTmpl, err)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(taskfileDir, path)
		}

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
			vars, err = eval.SourceShellFile(path, taskfileDir)
		} else {
			vars, err = parseDotenvFile(path)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("env file %q: %w", path, err)
		}

		for k, v := range vars {
			values[k] = v
			secrets[k] = isSecret
			scopeSoFar[k] = v
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
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		vars[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}
