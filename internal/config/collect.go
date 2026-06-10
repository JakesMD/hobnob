package config

import "strings"

func CollectGetParams(steps []Step, cfg *ConfigFile) []GetEntry {
	return collectGetParams(steps, cfg, make(map[string]bool), make(map[string]bool))
}

func collectGetParams(steps []Step, cfg *ConfigFile, visited map[string]bool, preset map[string]bool) []GetEntry {
	var entries []GetEntry
	alreadySet := make(map[string]bool, len(preset))
	for k := range preset {
		alreadySet[k] = true
	}
	for _, s := range steps {
		switch s.Kind {
		case KindSet:
			for _, e := range s.SetEntries {
				alreadySet[e.Key] = true
			}
		case KindGet:
			for _, e := range s.GetEntries {
				if !alreadySet[e.VarName] {
					entries = append(entries, e)
				}
				alreadySet[e.VarName] = true
			}
		case KindRun:
			for _, e := range s.IntoEntries {
				alreadySet[e.ParentKey] = true
			}
		case KindFor:
			loopPreset := make(map[string]bool, len(alreadySet))
			for k := range alreadySet {
				loopPreset[k] = true
			}
			if len(s.ForMatrix) > 0 {
				for _, matrixEntry := range s.ForMatrix {
					loopPreset[matrixEntry.VarName] = true
				}
			} else {
				loopPreset["ITEM"] = true
			}
			entries = append(entries, collectGetParams(s.ForSteps, cfg, visited, loopPreset)...)
		case KindCall:
			if cfg != nil && !strings.Contains(s.CallTarget, "{{") {
				if !visited[s.CallTarget] {
					childPreset := make(map[string]bool, len(alreadySet)+len(s.CallVars))
					for k := range alreadySet {
						childPreset[k] = true
					}
					for _, w := range s.CallVars {
						childPreset[w.Key] = true
					}
					visited[s.CallTarget] = true
					if task, ok := cfg.Tasks[s.CallTarget]; ok {
						for _, e := range collectGetParams(task.Steps, cfg, visited, childPreset) {
							if !alreadySet[e.VarName] {
								entries = append(entries, e)
							}
							alreadySet[e.VarName] = true
						}
					}
					delete(visited, s.CallTarget)
				}
				for _, e := range s.IntoEntries {
					alreadySet[e.ParentKey] = true
				}
			}
		}
	}
	return entries
}
