package scope

import (
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
// anything else is parsed as dotenv text (see parseDotenv). Later entries
// override earlier ones in the returned maps.
// A dotenv $NAME reference is resolved by ref, given the env file values so
// far (resolve passes Scope.envFileRef, keeping the ADR-0001 order there).
// Vars default to secret: false; an entry's own secret: true opts it into
// masking.
// A referenced file that doesn't exist prints a warning to stderr and is
// skipped, rather than failing the whole run — a typo'd optional env file
// shouldn't block every task.
func loadEnvFiles(ctx context.Context, entries []config.EnvFileEntry, taskfileDir string, scopeVars map[string]value.Value, ref func(name string, envSoFar map[string]string) (string, bool)) (values map[string]string, secrets map[string]bool, err error) {
	values = make(map[string]string)
	secrets = make(map[string]bool)
	scopeSoFar := eval.CloneMap(scopeVars)
	lookup := func(name string) (string, bool) { return ref(name, values) }
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

		isSecret := entry.SecretOverride != nil && *entry.SecretOverride
		assign := func(varName, varValue string) {
			values[varName] = varValue
			secrets[varName] = isSecret
			scopeSoFar[varName] = value.Str(varValue)
		}
		if strings.HasSuffix(path, ".sh") {
			var vars map[string]string
			vars, err = eval.SourceShellFile(ctx, path, taskfileDir)
			for varName, varValue := range vars {
				assign(varName, varValue)
			}
		} else {
			var data []byte
			if data, err = os.ReadFile(path); err == nil {
				err = parseDotenv(string(data), lookup, assign)
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("env file %q: %w", path, err)
		}
	}
	return values, secrets, nil
}
