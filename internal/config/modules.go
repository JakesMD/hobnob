package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"hobnob/internal/eval"
)

// LoadModules resolves module entries against scope, loads each module file,
// and merges the resulting tasks into cfg. Must be called after BuildScope.
// A module's own env: block is sourced for that module's own subtree only —
// same rule as vars: (see GUIDE.md Modules > Scoping), so it never leaks into
// the parent's vars/secrets.
func LoadModules(cfg *ConfigFile, vars map[string]string, secrets map[string]bool) error {
	ancestors := map[string]bool{}
	if cfg.FilePath != "" {
		ancestors[cfg.FilePath] = true
	}
	return loadModules(cfg, vars, secrets, ancestors)
}

func loadModules(cfg *ConfigFile, vars map[string]string, secrets map[string]bool, ancestors map[string]bool) error {
	for _, mod := range cfg.Modules {
		filePath, err := eval.EvalTemplate(mod.FileTmpl, vars)
		if err != nil {
			return fmt.Errorf("module %q file path: %w", mod.Prefix, err)
		}
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(cfg.TaskfileDir, filePath)
		}
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("module %q: resolve path: %w", mod.Prefix, err)
		}
		if ancestors[absPath] {
			return fmt.Errorf("module %q: circular import: %s", mod.Prefix, absPath)
		}

		modCfg, err := ParseConfig(filePath)
		if err != nil {
			return fmt.Errorf("module %q: %w", mod.Prefix, err)
		}

		// modVars/modSecrets stay local to this module's own subtree — a
		// module's env: file is private the same way its vars: block is.
		modVars := eval.CopyVars(vars)
		modSecrets := make(map[string]bool, len(secrets))
		for k, v := range secrets {
			modSecrets[k] = v
		}

		envVars, envSecrets, err := LoadEnvFiles(modCfg.EnvFileTmpls, modCfg.TaskfileDir, modVars)
		if err != nil {
			return fmt.Errorf("module %q: %w", mod.Prefix, err)
		}
		for k, v := range envVars {
			modVars[k] = v
			if envSecrets[k] {
				modSecrets[k] = true
			}
		}

		newAncestors := make(map[string]bool, len(ancestors)+1)
		for k := range ancestors {
			newAncestors[k] = true
		}
		newAncestors[absPath] = true

		if err := loadModules(modCfg, modVars, modSecrets, newAncestors); err != nil {
			return fmt.Errorf("module %q: %w", mod.Prefix, err)
		}

		showSet, err := evalStringSet(mod.ShowTmpls, vars)
		if err != nil {
			return fmt.Errorf("module %q show: %w", mod.Prefix, err)
		}
		hideSet, err := evalStringSet(mod.HideTmpls, vars)
		if err != nil {
			return fmt.Errorf("module %q hide: %w", mod.Prefix, err)
		}

		flatten := false
		if mod.FlattenTmpl != "" {
			flatVal, err := eval.EvalTemplate(mod.FlattenTmpl, vars)
			if err != nil {
				return fmt.Errorf("module %q flatten: %w", mod.Prefix, err)
			}
			flatten = flatVal == "true"
		}

		isInternal := strings.HasPrefix(mod.Prefix, "_")

		for _, taskName := range modCfg.TaskNames {
			if len(showSet) > 0 {
				if _, ok := showSet[taskName]; !ok {
					continue
				}
			}
			if _, ok := hideSet[taskName]; ok {
				continue
			}

			if strings.HasPrefix(taskName, "_") {
				continue
			}

			task := modCfg.Tasks[taskName]
			if task.Cfg == nil {
				task.Cfg = modCfg
			}
			if isInternal {
				task.Hidden = true
			}

			prefixedName := mod.Prefix + ":" + taskName

			if flatten && !isInternal {
				if _, exists := cfg.Tasks[taskName]; !exists {
					// flat alias becomes the visible name; prefixed is a hidden alias
					cfg.Tasks[taskName] = task
					cfg.TaskNames = append(cfg.TaskNames, taskName)

					taskHidden := task
					taskHidden.Hidden = true
					cfg.Tasks[prefixedName] = taskHidden
					cfg.TaskNames = append(cfg.TaskNames, prefixedName)
				} else {
					// collision with existing task — register prefixed name as visible
					cfg.Tasks[prefixedName] = task
					cfg.TaskNames = append(cfg.TaskNames, prefixedName)
				}
			} else {
				cfg.Tasks[prefixedName] = task
				cfg.TaskNames = append(cfg.TaskNames, prefixedName)
			}
		}
	}
	return nil
}

func evalStringSet(tmpls []string, scope map[string]string) (map[string]struct{}, error) {
	if len(tmpls) == 0 {
		return nil, nil
	}
	set := make(map[string]struct{}, len(tmpls))
	for _, tmpl := range tmpls {
		val, err := eval.EvalTemplate(tmpl, scope)
		if err != nil {
			return nil, err
		}
		set[val] = struct{}{}
	}
	return set, nil
}
