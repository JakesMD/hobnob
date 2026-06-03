package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"hobnob/internal/eval"
)

// LoadModules resolves module entries against scope, loads each module file,
// and merges the resulting tasks into cfg. Must be called after BuildScope.
func LoadModules(cfg *ConfigFile, scope map[string]string) error {
	ancestors := map[string]bool{}
	if cfg.FilePath != "" {
		ancestors[cfg.FilePath] = true
	}
	return loadModules(cfg, scope, ancestors)
}

func loadModules(cfg *ConfigFile, scope map[string]string, ancestors map[string]bool) error {
	for _, mod := range cfg.Modules {
		filePath, err := eval.EvalTemplate(mod.FileTmpl, scope)
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

		newAncestors := make(map[string]bool, len(ancestors)+1)
		for k := range ancestors {
			newAncestors[k] = true
		}
		newAncestors[absPath] = true

		if err := loadModules(modCfg, scope, newAncestors); err != nil {
			return fmt.Errorf("module %q: %w", mod.Prefix, err)
		}

		showSet, err := evalStringSet(mod.ShowTmpls, scope)
		if err != nil {
			return fmt.Errorf("module %q show: %w", mod.Prefix, err)
		}
		hideSet, err := evalStringSet(mod.HideTmpls, scope)
		if err != nil {
			return fmt.Errorf("module %q hide: %w", mod.Prefix, err)
		}

		flatten := false
		if mod.FlattenTmpl != "" {
			flatVal, err := eval.EvalTemplate(mod.FlattenTmpl, scope)
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

			t := modCfg.Tasks[taskName]
			if t.Cfg == nil {
				t.Cfg = modCfg
			}
			if isInternal {
				t.Hidden = true
			}

			prefixedName := mod.Prefix + ":" + taskName

			if flatten && !isInternal {
				if _, exists := cfg.Tasks[taskName]; !exists {
					// flat alias becomes the visible name; prefixed is a hidden alias
					cfg.Tasks[taskName] = t
					cfg.TaskNames = append(cfg.TaskNames, taskName)

					tHidden := t
					tHidden.Hidden = true
					cfg.Tasks[prefixedName] = tHidden
					cfg.TaskNames = append(cfg.TaskNames, prefixedName)
				} else {
					// collision with existing task — register prefixed name as visible
					cfg.Tasks[prefixedName] = t
					cfg.TaskNames = append(cfg.TaskNames, prefixedName)
				}
			} else {
				cfg.Tasks[prefixedName] = t
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
