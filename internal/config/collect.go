package config

import (
	"strings"

	"hobnob/internal/eval"
)

func CollectGetParams(steps []Step, cfg *ConfigFile) []GetEntry {
	return collectGetParams(steps, cfg, make(map[string]bool), make(map[string]bool))
}

func collectGetParams(steps []Step, cfg *ConfigFile, visited map[string]bool, preset map[string]bool) []GetEntry {
	var entries []GetEntry
	alreadySet := eval.CloneMap(preset)
	for _, step := range steps {
		switch step.Kind {
		case KindSet:
			for _, setEntry := range step.SetEntries {
				alreadySet[setEntry.Key] = true
			}
		case KindGet:
			for _, getEntry := range step.GetEntries {
				if !alreadySet[getEntry.VarName] {
					entries = append(entries, getEntry)
				}
				alreadySet[getEntry.VarName] = true
			}
		case KindRun:
			for _, intoEntry := range step.IntoEntries {
				alreadySet[intoEntry.ParentKey] = true
			}
		case KindFor:
			loopPreset := eval.CloneMap(alreadySet)
			if len(step.ForMatrix) > 0 {
				for _, matrixEntry := range step.ForMatrix {
					loopPreset[matrixEntry.VarName] = true
				}
			} else {
				loopPreset["ITEM"] = true
			}
			entries = append(entries, collectGetParams(step.ForSteps, cfg, visited, loopPreset)...)
		case KindCall:
			if cfg != nil && !strings.Contains(step.CallTarget, "{{") {
				if !visited[step.CallTarget] {
					childPreset := eval.CloneMap(alreadySet)
					for _, callVar := range step.CallVars {
						childPreset[callVar.Key] = true
					}
					visited[step.CallTarget] = true
					if task, ok := cfg.Tasks[step.CallTarget]; ok {
						for _, getEntry := range collectGetParams(task.Steps, cfg, visited, childPreset) {
							if !alreadySet[getEntry.VarName] {
								entries = append(entries, getEntry)
							}
							alreadySet[getEntry.VarName] = true
						}
					}
					delete(visited, step.CallTarget)
				}
				for _, intoEntry := range step.IntoEntries {
					alreadySet[intoEntry.ParentKey] = true
				}
			}
		}
	}
	return entries
}
