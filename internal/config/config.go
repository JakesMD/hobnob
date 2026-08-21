package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func ParseConfig(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read taskfile: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve taskfile path: %w", err)
	}
	return ParseConfigData(data, absPath, filepath.Dir(absPath))
}

// ParseConfigData parses taskfile bytes that never came from disk — the
// built-in demo taskfile the CLI falls back to when auto-discovery finds
// nothing. filePath is used only in error messages and HOBNOB_FILE_DIR's
// sibling FilePath; dir is what relative dir:/env:/modules: paths resolve
// against, so a caller with no real file on disk passes the invocation dir.
func ParseConfigData(data []byte, filePath, dir string) (*ConfigFile, error) {
	path := filePath
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse taskfile %s: %w", path, err)
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

	cfg := &ConfigFile{
		FilePath:    filePath,
		Tasks:       make(map[string]Task),
		TaskfileDir: dir,
	}

	var rootContent []*yaml.Node
	if root != nil && !isEmptyNode(root) {
		rootContent = root.Content
	}
	for _, entry := range mapEntries(rootContent) {
		key, val := entry.Key, entry.Val
		if isEmptyNode(val) {
			continue
		}
		switch key {
		case "env":
			paths, err := parseEnvNode(val)
			if err != nil {
				return nil, fmt.Errorf("env: %w", err)
			}
			cfg.EnvFileTmpls = paths
		case "const":
			entries, err := parseSetNode(val)
			if err != nil {
				return nil, fmt.Errorf("const: %w", err)
			}
			cfg.ConstEntries = entries
		case "vars":
			entries, err := parseSetNode(val)
			if err != nil {
				return nil, fmt.Errorf("vars: %w", err)
			}
			cfg.VarEntries = entries
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
			for _, taskEntry := range mapEntries(val.Content) {
				taskName, taskNode := taskEntry.Key, taskEntry.Val
				task, err := parseTaskNode(taskNode)
				if err != nil {
					return nil, fmt.Errorf("task %q: %w", taskName, err)
				}
				cfg.Tasks[taskName] = task
				cfg.TaskNames = append(cfg.TaskNames, taskName)
			}
		default:
			return nil, fmt.Errorf("unknown top-level key %q", key)
		}
	}

	if err := checkConstClosedWorld(cfg.ConstEntries); err != nil {
		return nil, err
	}
	if err := checkVarsNoSelfReference(cfg.VarEntries); err != nil {
		return nil, err
	}
	if err := checkConstNamesNotShadowed(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseTaskNode(node *yaml.Node) (Task, error) {
	if node.Kind != yaml.MappingNode {
		return Task{}, fmt.Errorf("task must be a mapping")
	}
	var task Task
	for _, entry := range mapEntries(node.Content) {
		switch entry.Key {
		case "info":
			task.Info = entry.Val.Value
		case "if":
			task.IfExpr = entry.Val.Value
		case "dir":
			task.Dir = normalizeTmpl(entry.Val.Value)
		case "once":
			task.Once = parseBool(entry.Val)
		case "steps":
			steps, err := parseStepSequence(entry.Val)
			if err != nil {
				return Task{}, err
			}
			task.Steps = steps
		}
	}
	return task, nil
}

func parseStepSequence(node *yaml.Node) ([]Step, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("steps must be a sequence")
	}
	var steps []Step
	for _, item := range node.Content {
		step, err := parseStepNode(item)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}
