package config

import "fmt"

// checkConstNamesNotShadowed rejects a set:/get:/into:/with:/loop: target
// (loop: iterators included) anywhere in cfg's own tasks that collides with
// one of cfg's own const: names. Without this, const: would only be constant from outside the
// file — the timeline still outranks it in the precedence chain, so a
// task's own set: could quietly overwrite one. Scoped per-file: a module's
// tasks are checked against the module's own const: block, not the
// parent's, matching every other file-scoped rule (env:, vars:, const:
// itself).
func checkConstNamesNotShadowed(cfg *ConfigFile) error {
	if len(cfg.ConstEntries) == 0 {
		return nil
	}
	names := make(map[string]bool, len(cfg.ConstEntries))
	for _, entry := range cfg.ConstEntries {
		names[entry.Key] = true
	}
	for _, taskName := range cfg.TaskNames {
		if err := checkStepsDontShadow(taskName, cfg.Tasks[taskName].Steps, names, cfg.Tasks); err != nil {
			return err
		}
	}
	return nil
}

// checkStepsDontShadow asks each step what it writes (Step.Writes). A call:'s
// with: keys count whatever its target, since they'd outrank the const: in
// the called task's scope. A call: to a literal target in this same file
// counts its results: (see CallInto); a templated or module target isn't
// known at load time, so runner.finishCall checks it when the call runs.
func checkStepsDontShadow(taskName string, steps []Step, names map[string]bool, tasks map[string]Task) error {
	for _, step := range steps {
		for _, name := range step.Writes(tasks) {
			if names[name] {
				return fmt.Errorf("task %s sets %s, declared in const: — pick another name", taskName, name)
			}
		}
	}
	return nil
}
