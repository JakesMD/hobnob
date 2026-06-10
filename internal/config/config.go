package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// bareVarRef matches a bare .VAR reference with an optional pipe chain,
// e.g. ".RELEASE_LIST" or ".RELEASE_LIST | first". Relative paths (./infra,
// ../tests) and dotfiles (.git) never match — they contain "/" or a
// lowercase-leading segment, which this pattern excludes.
var bareVarRef = regexp.MustCompile(`^\.[A-Z][A-Z0-9_]*(\s*\|[^{]+)?$`)

// normalizeTmpl wraps a bare .VAR or .VAR | filter expression in {{ }} so
// users can write `default: .LIST | first` instead of `default: '{{.LIST | first}}'`.
func normalizeTmpl(s string) string {
	if bareVarRef.MatchString(s) {
		return "{{" + s + "}}"
	}
	return s
}

type Task struct {
	Info   string
	Dir    string // task-level working directory template
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
	FilePath    string
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
		FilePath:    absPath,
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
	var task Task
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch n.Content[i].Value {
		case "info":
			task.Info = n.Content[i+1].Value
		case "dir":
			task.Dir = normalizeTmpl(n.Content[i+1].Value)
		case "steps":
			steps, err := parseStepSequence(n.Content[i+1])
			if err != nil {
				return Task{}, err
			}
			task.Steps = steps
		}
	}
	return task, nil
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
		fieldKey := n.Content[i].Value
		fieldVal := n.Content[i+1]

		switch fieldKey {
		case "run":
			s.Kind = KindRun
			s.Command = normalizeTmpl(fieldVal.Value)
		case "set":
			s.Kind = KindSet
			entries, err := parseSetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("set: %w", err)
			}
			s.SetEntries = entries
		case "get":
			s.Kind = KindGet
			entries, err := parseGetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("get: %w", err)
			}
			s.GetEntries = entries
		case "call":
			s.Kind = KindCall
			s.CallTarget = fieldVal.Value
		case "loop":
			s.Kind = KindFor
			switch fieldVal.Kind {
			case yaml.SequenceNode:
				for _, item := range fieldVal.Content {
					s.ForList = append(s.ForList, item.Value)
				}
			case yaml.MappingNode:
				for i := 0; i+1 < len(fieldVal.Content); i += 2 {
					varName := fieldVal.Content[i].Value
					if strings.Contains(varName, "{{") {
						return s, fmt.Errorf("variable name %q must not contain template syntax", varName)
					}
					valNode := fieldVal.Content[i+1]
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
			default:
				s.ForTarget = normalizeTmpl(fieldVal.Value)
			}
		case "if":
			s.IfExpr = fieldVal.Value
		case "with":
			entries, err := parseSetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("with: %w", err)
			}
			s.CallVars = entries
		case "into":
			entries, err := parseIntoNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("into: %w", err)
			}
			s.IntoEntries = entries
		case "soft":
			s.Soft = fieldVal.Value == "true"
		case "dir":
			s.DirTmpl = normalizeTmpl(fieldVal.Value)
		case "steps":
			subSteps, err := parseStepSequence(fieldVal)
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
			entry := SetEntry{Key: key}
			for j := 0; j+1 < len(valNode.Content); j += 2 {
				switch valNode.Content[j].Value {
				case "value":
					entry.ValTmpl = normalizeTmpl(valNode.Content[j+1].Value)
				case "secret":
					entry.Secret = valNode.Content[j+1].Value == "true"
				}
			}
			entries = append(entries, entry)
			continue
		}
		valTmpl := normalizeTmpl(valNode.Value)
		if valNode.Kind == yaml.SequenceNode {
			items := make([]string, len(valNode.Content))
			for i, child := range valNode.Content {
				items[i] = child.Value
			}
			jsonBytes, err := json.Marshal(items)
			if err != nil {
				return nil, fmt.Errorf("set entry %q: failed to serialize list: %w", item.Content[0].Value, err)
			}
			valTmpl = string(jsonBytes)
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
			fieldKey := modifiers.Content[i].Value
			fieldVal := modifiers.Content[i+1]
			switch fieldKey {
			case "info":
				e.Info = fieldVal.Value
			case "options":
				if fieldVal.Kind == yaml.SequenceNode {
					for _, fi := range fieldVal.Content {
						e.FromList = append(e.FromList, fi.Value)
					}
				} else {
					e.FromTmpl = normalizeTmpl(fieldVal.Value)
				}
			case "multi":
				e.Multi = fieldVal.Value == "true"
			case "check":
				e.Check = fieldVal.Value
			case "default":
				e.DefaultTmpl = normalizeTmpl(fieldVal.Value)
			case "secret":
				e.Secret = fieldVal.Value == "true"
			case "optional":
				e.Optional = fieldVal.Value == "true"
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// applyModuleFlag applies a show/hide/flatten key-value pair to m.
// Used for both sibling-key form (- docker: ./path\n  show: [...]) and
// nested object form (- docker: {path: ..., show: [...]}).
func applyModuleFlag(m *ModuleEntry, key string, val *yaml.Node) error {
	switch key {
	case "show":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module show must be a sequence")
		}
		for _, child := range val.Content {
			m.ShowTmpls = append(m.ShowTmpls, child.Value)
		}
	case "hide":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module hide must be a sequence")
		}
		for _, child := range val.Content {
			m.HideTmpls = append(m.HideTmpls, child.Value)
		}
	case "flatten":
		m.FlattenTmpl = val.Value
	}
	return nil
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
		var mod ModuleEntry
		for i := 0; i+1 < len(item.Content); i += 2 {
			key := item.Content[i].Value
			val := item.Content[i+1]
			// "show", "hide", "flatten" are reserved at the top-level mapping key
			// position. Using them as module prefixes always produces a parse error
			// (not a silent shadow): "show"/"hide" fail the sequence check; "flatten"
			// sets FlattenTmpl, leaving Prefix empty → "module entry missing prefix key".
			switch key {
			case "show", "hide", "flatten":
				if err := applyModuleFlag(&mod, key, val); err != nil {
					return nil, err
				}
			default:
				mod.Prefix = key
				if val.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(val.Content); j += 2 {
						subKey := val.Content[j].Value
						subVal := val.Content[j+1]
						if subKey == "path" {
							mod.FileTmpl = subVal.Value
						} else if err := applyModuleFlag(&mod, subKey, subVal); err != nil {
							return nil, err
						}
					}
				} else {
					mod.FileTmpl = val.Value
				}
			}
		}
		if mod.Prefix == "" {
			return nil, fmt.Errorf("module entry missing prefix key")
		}
		mods = append(mods, mod)
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
