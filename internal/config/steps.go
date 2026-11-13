package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"hobnob/internal/literal"
)

func parseStepNode(node *yaml.Node) (Step, error) {
	if node.Kind != yaml.MappingNode {
		return Step{}, fmt.Errorf("step must be a mapping")
	}

	entries := mapEntries(node.Content)
	kind, err := stepKindOf(entries)
	if err != nil {
		return Step{}, err
	}
	step := Step{Kind: kind}

	for _, entry := range entries {
		fieldKey, fieldVal := entry.Key, entry.Val

		switch fieldKey {
		case "run":
			switch fieldVal.Kind {
			case yaml.SequenceNode:
				argv, err := literal.ParseArgv(fieldVal)
				if err != nil {
					return Step{}, err
				}
				step.Argv = argv
			case yaml.MappingNode:
				return Step{}, fmt.Errorf("run: expected a command string or a list of arguments, got a mapping")
			default:
				cmd, err := literal.Single(fieldVal.Value)
				if err != nil {
					return Step{}, fmt.Errorf("run: %w", err)
				}
				step.Command = cmd
			}
		case "set":
			entries, err := parseSetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("set: %w", err)
			}
			step.SetEntries = entries
		case "get":
			entries, err := parseGetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("get: %w", err)
			}
			step.GetEntries = entries
		case "call":
			step.CallTarget = fieldVal.Value
		case "loop":
			forTarget, forList, forMatrix, err := parseLoopNode(fieldVal)
			if err != nil {
				return step, err
			}
			step.ForTarget, step.ForList, step.ForMatrix = forTarget, forList, forMatrix
		case "if":
			step.IfExpr = fieldVal.Value
		case "with":
			entries, err := parseSetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("with: %w", err)
			}
			if err := rejectSecretCallVars(entries); err != nil {
				return Step{}, err
			}
			step.CallVars = entries
		case "into":
			entries, err := parseIntoNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("into: %w", err)
			}
			step.IntoEntries = entries
			step.HasInto = true
		case "soft":
			step.Soft = parseBool(fieldVal)
		case "dir":
			dir, err := literal.Single(fieldVal.Value)
			if err != nil {
				return Step{}, fmt.Errorf("dir: %w", err)
			}
			step.DirTmpl = dir
		case "quiet":
			if fieldVal.Kind != yaml.ScalarNode {
				return Step{}, fmt.Errorf("quiet: expected true/false or a message string")
			}
			if fieldVal.Value == "true" || fieldVal.Value == "false" {
				step.Quiet = parseBool(fieldVal)
			} else {
				step.Quiet = true
				msg, err := literal.Single(fieldVal.Value)
				if err != nil {
					return Step{}, fmt.Errorf("quiet: %w", err)
				}
				step.QuietMsg = msg
			}
		case "steps":
			subSteps, err := parseStepSequence(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("steps: %w", err)
			}
			step.ForSteps = subSteps
		}
	}

	if step.Kind == KindRun {
		for _, entry := range step.IntoEntries {
			if entry.Bare {
				return Step{}, fmt.Errorf("into: bare entry %q only works on call: steps; on run: write %s: stdout", entry.ParentKey, entry.ParentKey)
			}
		}
	}

	return step, nil
}

// rejectSecretCallVars rejects secret: on a with: entry. with: reuses set:'s
// grammar, so the field parses, but marking a var secret at the call site is
// never the right tool: masking matches on value, and Scope.Copy() carries the
// parent's secrets into the child, so a secret passed down is already masked
// under its new name. Flagging it here only over-masks — a composed value like
// "postgres://{{.USER}}:{{.PASS}}@db" would blank out entirely instead of just
// the password. Declare secrecy where the value originates (set:/get:/env:) and
// let it propagate.
func rejectSecretCallVars(entries []SetEntry) error {
	for _, entry := range entries {
		if entry.Secret {
			return fmt.Errorf("with entry %q: secret: is not supported here; mark the variable secret where it's defined (set:, get: or env:) — it stays masked when passed through with:", entry.Key)
		}
	}
	return nil
}

func parseLoopNode(fieldVal *yaml.Node) (forTarget literal.Expr, forList []string, forMatrix []ForMatrixEntry, err error) {
	switch fieldVal.Kind {
	case yaml.SequenceNode:
		for _, item := range fieldVal.Content {
			forList = append(forList, item.Value)
		}
	case yaml.MappingNode:
		for _, mapEntry := range mapEntries(fieldVal.Content) {
			varName := mapEntry.Key
			if err := validateVarName(varName); err != nil {
				return literal.Expr{}, nil, nil, err
			}
			valNode := mapEntry.Val
			entry := ForMatrixEntry{VarName: varName}
			if valNode.Kind == yaml.SequenceNode {
				for _, item := range valNode.Content {
					entry.List = append(entry.List, item.Value)
				}
			} else {
				tmpl, err := literal.Single(valNode.Value)
				if err != nil {
					return literal.Expr{}, nil, nil, fmt.Errorf("loop: %s: %w", varName, err)
				}
				entry.ListTmpl = tmpl
			}
			forMatrix = append(forMatrix, entry)
		}
	default:
		target, err := literal.Single(fieldVal.Value)
		if err != nil {
			return literal.Expr{}, nil, nil, fmt.Errorf("loop: %w", err)
		}
		forTarget = target
	}
	return forTarget, forList, forMatrix, nil
}
