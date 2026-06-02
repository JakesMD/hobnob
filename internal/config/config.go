package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// normalizeTmpl wraps a bare .VAR reference in {{ }} so users can write
// `options: .RELEASE_LIST` instead of `options: '{{.RELEASE_LIST}}'`.
func normalizeTmpl(s string) string {
	if strings.HasPrefix(s, ".") && !strings.Contains(s, "{{") {
		return "{{" + s + "}}"
	}
	return s
}

type Task struct {
	Info   string
	Dir    string      // task-level working directory template
	Steps  []Step
	Hidden bool        // true = omit from --list
	Cfg    *ConfigFile // non-nil = use this cfg for sub-calls (module task)
}

type ModuleEntry struct {
	Prefix      string
	FileTmpl    string
	ShowTmpls   []string
	HideTmpls   []string
	FlattenTmpl string
}

type ConfigFile struct {
	Vars        []SetEntry
	Tasks       map[string]Task
	TaskNames   []string
	TaskfileDir string
	Modules     []ModuleEntry
}

type StepKind int

const (
	KindRun StepKind = iota
	KindSet
	KindCall
	KindFor
	KindGet
)

type SetEntry struct {
	Key     string
	ValTmpl string
	Secret  bool
}

type IntoEntry struct {
	ParentKey string
	ValueTmpl string
}

type GetEntry struct {
	VarName     string
	Info        string
	FromList    []string
	FromTmpl    string
	Multi       bool
	Check       string
	DefaultTmpl string
	Secret      bool
	Optional    bool
}

type ForMatrixEntry struct {
	VarName  string
	List     []string
	ListTmpl string
}

type Step struct {
	Kind   StepKind
	IfExpr string

	// KindSet
	SetEntries []SetEntry

	// KindRun
	Command string
	DirTmpl string // working directory template (run: and call: steps)

	// KindCall
	CallTarget  string
	CallVars    []SetEntry
	IntoEntries []IntoEntry
	Soft        bool

	// KindFor
	ForTarget string
	ForList   []string
	ForVar    string
	ForMatrix []ForMatrixEntry
	ForSteps  []Step

	// KindGet
	GetEntries []GetEntry
}

func ParseConfig(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("root must be a mapping")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve taskfile path: %w", err)
	}
	cfg := &ConfigFile{
		Tasks:       make(map[string]Task),
		TaskfileDir: filepath.Dir(absPath),
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		val := root.Content[i+1]
		switch key {
		case "vars":
			entries, err := parseSetNode(val)
			if err != nil {
				return nil, fmt.Errorf("vars: %w", err)
			}
			cfg.Vars = entries
		case "modules":
			mods, err := parseModulesNode(val)
			if err != nil {
				return nil, fmt.Errorf("modules: %w", err)
			}
			cfg.Modules = mods
		case "tasks":
			if val.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("tasks must be a mapping")
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				taskName := val.Content[j].Value
				taskNode := val.Content[j+1]
				t, err := parseTaskNode(taskNode)
				if err != nil {
					return nil, fmt.Errorf("task %q: %w", taskName, err)
				}
				cfg.Tasks[taskName] = t
				cfg.TaskNames = append(cfg.TaskNames, taskName)
			}
		}
	}
	return cfg, nil
}

func parseTaskNode(n *yaml.Node) (Task, error) {
	if n.Kind != yaml.MappingNode {
		return Task{}, fmt.Errorf("task must be a mapping")
	}
	var t Task
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch n.Content[i].Value {
		case "info":
			t.Info = n.Content[i+1].Value
		case "dir":
			t.Dir = n.Content[i+1].Value
		case "steps":
			steps, err := parseStepSequence(n.Content[i+1])
			if err != nil {
				return Task{}, err
			}
			t.Steps = steps
		}
	}
	return t, nil
}

func parseStepSequence(n *yaml.Node) ([]Step, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("steps must be a sequence")
	}
	var steps []Step
	for _, item := range n.Content {
		s, err := parseStepNode(item)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, nil
}

func parseStepNode(n *yaml.Node) (Step, error) {
	if n.Kind != yaml.MappingNode {
		return Step{}, fmt.Errorf("step must be a mapping")
	}

	var s Step

	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		v := n.Content[i+1]

		switch k {
		case "run":
			s.Kind = KindRun
			s.Command = v.Value
		case "set":
			s.Kind = KindSet
			entries, err := parseSetNode(v)
			if err != nil {
				return Step{}, fmt.Errorf("set: %w", err)
			}
			s.SetEntries = entries
		case "get":
			s.Kind = KindGet
			entries, err := parseGetNode(v)
			if err != nil {
				return Step{}, fmt.Errorf("get: %w", err)
			}
			s.GetEntries = entries
		case "call":
			s.Kind = KindCall
			s.CallTarget = v.Value
		case "loop":
			s.Kind = KindFor
			if v.Kind == yaml.SequenceNode {
				for _, item := range v.Content {
					s.ForList = append(s.ForList, item.Value)
				}
			} else if v.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(v.Content); i += 2 {
					varName := v.Content[i].Value
					if strings.Contains(varName, "{{") {
						return s, fmt.Errorf("variable name %q must not contain template syntax", varName)
					}
					valNode := v.Content[i+1]
					entry := ForMatrixEntry{VarName: varName}
					if valNode.Kind == yaml.SequenceNode {
						for _, item := range valNode.Content {
							entry.List = append(entry.List, item.Value)
						}
					} else {
						entry.ListTmpl = normalizeTmpl(valNode.Value)
					}
					s.ForMatrix = append(s.ForMatrix, entry)
				}
			} else {
				s.ForTarget = normalizeTmpl(v.Value)
			}
		case "if":
			s.IfExpr = v.Value
		case "with":
			entries, err := parseSetNode(v)
			if err != nil {
				return Step{}, fmt.Errorf("with: %w", err)
			}
			s.CallVars = entries
		case "into":
			entries, err := parseIntoNode(v)
			if err != nil {
				return Step{}, fmt.Errorf("into: %w", err)
			}
			s.IntoEntries = entries
		case "soft":
			s.Soft = v.Value == "true"
		case "dir":
			s.DirTmpl = v.Value
		case "as":
			s.ForVar = v.Value
		case "steps":
			subSteps, err := parseStepSequence(v)
			if err != nil {
				return Step{}, fmt.Errorf("steps: %w", err)
			}
			s.ForSteps = subSteps
		}
	}

	return s, nil
}

func parseSetNode(n *yaml.Node) ([]SetEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("set must be a sequence of key-value maps")
	}
	var entries []SetEntry
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each set entry must be a single key: value pair")
		}
		valNode := item.Content[1]
		if valNode.Kind == yaml.MappingNode {
			// expanded form: { value: ..., secret: true }
			key := item.Content[0].Value
			if strings.Contains(key, "{{") {
				return nil, fmt.Errorf("variable name %q must not contain template syntax", key)
			}
			e := SetEntry{Key: key}
			for j := 0; j+1 < len(valNode.Content); j += 2 {
				switch valNode.Content[j].Value {
				case "value":
					e.ValTmpl = valNode.Content[j+1].Value
				case "secret":
					e.Secret = valNode.Content[j+1].Value == "true"
				}
			}
			entries = append(entries, e)
			continue
		}
		valTmpl := valNode.Value
		if valNode.Kind == yaml.SequenceNode {
			items := make([]string, len(valNode.Content))
			for i, child := range valNode.Content {
				items[i] = child.Value
			}
			b, err := json.Marshal(items)
			if err != nil {
				return nil, fmt.Errorf("set entry %q: failed to serialize list: %w", item.Content[0].Value, err)
			}
			valTmpl = string(b)
		}
		rawKey := item.Content[0].Value
		if strings.Contains(rawKey, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", rawKey)
		}
		entries = append(entries, SetEntry{
			Key:     rawKey,
			ValTmpl: valTmpl,
		})
	}
	return entries, nil
}

func parseGetNode(n *yaml.Node) ([]GetEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("get must be a sequence")
	}
	var entries []GetEntry
	for _, item := range n.Content {
		if item.Kind == yaml.ScalarNode {
			if strings.Contains(item.Value, "{{") {
				return nil, fmt.Errorf("variable name %q must not contain template syntax", item.Value)
			}
			entries = append(entries, GetEntry{VarName: item.Value})
			continue
		}
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each get entry must be a single key: modifiers pair")
		}
		varName := item.Content[0].Value
		if strings.Contains(varName, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", varName)
		}
		e := GetEntry{VarName: varName}
		modifiers := item.Content[1]
		if modifiers.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("get entry %q modifiers must be a mapping", e.VarName)
		}
		for i := 0; i+1 < len(modifiers.Content); i += 2 {
			k := modifiers.Content[i].Value
			v := modifiers.Content[i+1]
			switch k {
			case "info":
				e.Info = v.Value
			case "options":
				if v.Kind == yaml.SequenceNode {
					for _, fi := range v.Content {
						e.FromList = append(e.FromList, fi.Value)
					}
				} else {
					e.FromTmpl = normalizeTmpl(v.Value)
				}
			case "multi":
				e.Multi = v.Value == "true"
			case "check":
				e.Check = v.Value
			case "default":
				e.DefaultTmpl = v.Value
			case "secret":
				e.Secret = v.Value == "true"
			case "optional":
				e.Optional = v.Value == "true"
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func parseModulesNode(n *yaml.Node) ([]ModuleEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("modules must be a sequence")
	}
	var mods []ModuleEntry
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("module entry must be a mapping")
		}
		var m ModuleEntry
		for i := 0; i+1 < len(item.Content); i += 2 {
			key := item.Content[i].Value
			val := item.Content[i+1]
			switch key {
			case "show":
				if val.Kind != yaml.SequenceNode {
					return nil, fmt.Errorf("module show must be a sequence")
				}
				for _, child := range val.Content {
					m.ShowTmpls = append(m.ShowTmpls, child.Value)
				}
			case "hide":
				if val.Kind != yaml.SequenceNode {
					return nil, fmt.Errorf("module hide must be a sequence")
				}
				for _, child := range val.Content {
					m.HideTmpls = append(m.HideTmpls, child.Value)
				}
			case "flatten":
				m.FlattenTmpl = val.Value
			default:
				m.Prefix = key
				if val.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(val.Content); j += 2 {
						subKey := val.Content[j].Value
						subVal := val.Content[j+1]
						switch subKey {
						case "path":
							m.FileTmpl = subVal.Value
						case "show":
							if subVal.Kind != yaml.SequenceNode {
								return nil, fmt.Errorf("module show must be a sequence")
							}
							for _, child := range subVal.Content {
								m.ShowTmpls = append(m.ShowTmpls, child.Value)
							}
						case "hide":
							if subVal.Kind != yaml.SequenceNode {
								return nil, fmt.Errorf("module hide must be a sequence")
							}
							for _, child := range subVal.Content {
								m.HideTmpls = append(m.HideTmpls, child.Value)
							}
						case "flatten":
							m.FlattenTmpl = subVal.Value
						}
					}
				} else {
					m.FileTmpl = val.Value
				}
			}
		}
		if m.Prefix == "" {
			return nil, fmt.Errorf("module entry missing prefix key")
		}
		mods = append(mods, m)
	}
	return mods, nil
}

func parseIntoNode(n *yaml.Node) ([]IntoEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("into must be a sequence of key: value maps")
	}
	var entries []IntoEntry
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, fmt.Errorf("each into entry must be a single key: value pair")
		}
		parentKey := item.Content[0].Value
		if strings.Contains(parentKey, "{{") {
			return nil, fmt.Errorf("variable name %q must not contain template syntax", parentKey)
		}
		entries = append(entries, IntoEntry{
			ParentKey: parentKey,
			ValueTmpl: item.Content[1].Value,
		})
	}
	return entries, nil
}
