package config

import (
	"fmt"
	"strings"

	"hobnob/internal/eval"
	"hobnob/internal/value"

	"gopkg.in/yaml.v3"
)

// applyModuleFlag applies a show/hide/flatten key-value pair to m.
// Used for both sibling-key form (- docker: ./path\n  show: [...]) and
// nested object form (- docker: {path: ..., show: [...]}).
func applyModuleFlag(module *ModuleEntry, key string, val *yaml.Node) error {
	switch key {
	case "show":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module show must be a sequence")
		}
		for _, child := range val.Content {
			module.ShowTmpls = append(module.ShowTmpls, child.Value)
		}
	case "hide":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module hide must be a sequence")
		}
		for _, child := range val.Content {
			module.HideTmpls = append(module.HideTmpls, child.Value)
		}
	case "flatten":
		module.FlattenTmpl = val.Value
	}
	return nil
}

func parseModulesNode(node *yaml.Node) ([]ModuleEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("modules must be a sequence")
	}
	var moduleEntries []ModuleEntry
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("module entry must be a mapping")
		}
		var module ModuleEntry
		for _, entry := range mapEntries(item.Content) {
			key, val := entry.Key, entry.Val
			// "show", "hide", "flatten" are reserved at the top-level mapping key
			// position. Using them as module prefixes always produces a parse error
			// (not a silent shadow): "show"/"hide" fail the sequence check; "flatten"
			// sets FlattenTmpl, leaving Prefix empty → "module entry missing prefix key".
			switch key {
			case "show", "hide", "flatten":
				if err := applyModuleFlag(&module, key, val); err != nil {
					return nil, err
				}
			default:
				module.Prefix = key
				if val.Kind == yaml.MappingNode {
					for _, sub := range mapEntries(val.Content) {
						if sub.Key == "path" {
							module.FileTmpl = sub.Val.Value
						} else if err := applyModuleFlag(&module, sub.Key, sub.Val); err != nil {
							return nil, err
						}
					}
				} else {
					module.FileTmpl = val.Value
				}
			}
		}
		if module.Prefix == "" {
			return nil, fmt.Errorf("module entry missing prefix key")
		}
		moduleEntries = append(moduleEntries, module)
	}
	return moduleEntries, nil
}

// RegisterModuleTasks applies module's show/hide/flatten filters to moduleCfg's
// tasks and registers the survivors into cfg under their prefixed (and,
// if flattened, bare) names.
func RegisterModuleTasks(cfg *ConfigFile, module ModuleEntry, moduleCfg *ConfigFile, vars map[string]value.Value) error {
	showSet, err := evalStringSet(module.ShowTmpls, vars)
	if err != nil {
		return fmt.Errorf("module %q show: %w", module.Prefix, err)
	}
	hideSet, err := evalStringSet(module.HideTmpls, vars)
	if err != nil {
		return fmt.Errorf("module %q hide: %w", module.Prefix, err)
	}

	flatten := false
	if module.FlattenTmpl != "" {
		flatVal, err := eval.EvalTemplate(module.FlattenTmpl, vars)
		if err != nil {
			return fmt.Errorf("module %q flatten: %w", module.Prefix, err)
		}
		flatten = flatVal == "true"
	}

	isInternal := strings.HasPrefix(module.Prefix, "_")

	for _, taskName := range moduleCfg.TaskNames {
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

		task := moduleCfg.Tasks[taskName]
		if task.Cfg == nil {
			task.Cfg = moduleCfg
		}
		if isInternal {
			task.Hidden = true
		}

		prefixedName := module.Prefix + ":" + taskName

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
	return nil
}

func evalStringSet(tmpls []string, scope map[string]value.Value) (map[string]struct{}, error) {
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
