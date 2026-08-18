package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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

// isEmptyNode reports whether n is a YAML null (a key given with no value,
// e.g. "vars:" followed by nothing, or explicit "vars: ~"/"vars: null").
// Top-level vars:/modules:/tasks: blocks tolerate this as "no entries".
func isEmptyNode(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// parseBool reports whether n's scalar value is the literal string "true" —
// hobnob's YAML bool fields (interactive, secret, multi, soft, optional)
// only ever hold unquoted true/false, so any other value (including a
// template) is simply false rather than an error.
func parseBool(n *yaml.Node) bool {
	return n.Value == "true"
}

type Task struct {
	Info   string
	Dir    string // task-level working directory template
	IfExpr string
	Interactive *bool
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
	FilePath     string
	Vars         []SetEntry
	EnvFileTmpls []EnvFileEntry
	Tasks        map[string]Task
	TaskNames    []string
	TaskfileDir  string
	Modules      []ModuleEntry
}

// EnvFileEntry is one env: block entry. SecretOverride is nil unless the
// entry explicitly sets secret: true/false; nil means "use the default,
// secret: false" (see config.LoadEnvFiles).
type EnvFileEntry struct {
	PathTmpl       string
	SecretOverride *bool
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
	Interactive *bool

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
	// An empty file (or one with only comments/whitespace) yields no document
	// node at all; a file containing just "null"/"~" yields a null root node.
	// Both mean "no tasks", same as an explicit empty root mapping.
	var root *yaml.Node
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root != nil && !isEmptyNode(root) && root.Kind != yaml.MappingNode {
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

	var rootContent []*yaml.Node
	if root != nil && !isEmptyNode(root) {
		rootContent = root.Content
	}
	for i := 0; i+1 < len(rootContent); i += 2 {
		key := rootContent[i].Value
		val := rootContent[i+1]
		if isEmptyNode(val) {
			continue
		}
		switch key {
		case "vars":
			entries, err := parseSetNode(val)
			if err != nil {
				return nil, fmt.Errorf("vars: %w", err)
			}
			cfg.Vars = entries
		case "env":
			paths, err := parseEnvNode(val)
			if err != nil {
				return nil, fmt.Errorf("env: %w", err)
			}
			cfg.EnvFileTmpls = paths
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
		case "if":
			task.IfExpr = n.Content[i+1].Value
		case "dir":
			task.Dir = normalizeTmpl(n.Content[i+1].Value)
		case "interactive":
			v := parseBool(n.Content[i+1])
			task.Interactive = &v
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

func parseEnvNode(n *yaml.Node) ([]EnvFileEntry, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("env must be a sequence of file paths")
	}
	var entries []EnvFileEntry
	for _, item := range n.Content {
		if item.Kind == yaml.ScalarNode {
			entries = append(entries, EnvFileEntry{PathTmpl: item.Value})
			continue
		}
		// expanded form: - path: { secret: false }
		if item.Kind != yaml.MappingNode || len(item.Content) != 2 {
			return nil, fmt.Errorf("each env entry must be a file path or a single path: modifiers pair")
		}
		entry := EnvFileEntry{PathTmpl: item.Content[0].Value}
		modifiers := item.Content[1]
		if modifiers.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("env entry %q modifiers must be a mapping", entry.PathTmpl)
		}
		for i := 0; i+1 < len(modifiers.Content); i += 2 {
			if modifiers.Content[i].Value == "secret" {
				v := parseBool(modifiers.Content[i+1])
				entry.SecretOverride = &v
			}
		}
		entries = append(entries, entry)
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
