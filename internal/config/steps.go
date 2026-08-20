package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func parseStepNode(node *yaml.Node) (Step, error) {
	if node.Kind != yaml.MappingNode {
		return Step{}, fmt.Errorf("step must be a mapping")
	}

	var step Step

	for _, entry := range mapEntries(node.Content) {
		fieldKey, fieldVal := entry.Key, entry.Val

		switch fieldKey {
		case "run":
			step.Kind = KindRun
			switch fieldVal.Kind {
			case yaml.SequenceNode:
				if len(fieldVal.Content) == 0 {
					return Step{}, fmt.Errorf("run: list form needs at least one element")
				}
				for _, item := range fieldVal.Content {
					step.Argv = append(step.Argv, normalizeTmpl(item.Value))
				}
			case yaml.MappingNode:
				return Step{}, fmt.Errorf("run: expected a command string or a list of arguments, got a mapping")
			default:
				step.Command = normalizeTmpl(fieldVal.Value)
			}
		case "set":
			step.Kind = KindSet
			entries, err := parseSetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("set: %w", err)
			}
			step.SetEntries = entries
		case "get":
			step.Kind = KindGet
			entries, err := parseGetNode(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("get: %w", err)
			}
			step.GetEntries = entries
		case "call":
			step.Kind = KindCall
			step.CallTarget = fieldVal.Value
		case "use":
			step.Kind = KindUse
			step.CallTarget = fieldVal.Value
		case "loop":
			step.Kind = KindFor
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
		case "soft":
			step.Soft = parseBool(fieldVal)
		case "interactive":
			interactive := parseBool(fieldVal)
			step.Interactive = &interactive
		case "dir":
			step.DirTmpl = normalizeTmpl(fieldVal.Value)
		case "rerun":
			step.Rerun = parseBool(fieldVal)
		case "steps":
			subSteps, err := parseStepSequence(fieldVal)
			if err != nil {
				return Step{}, fmt.Errorf("steps: %w", err)
			}
			step.ForSteps = subSteps
		}
	}

	if step.Kind == KindUse {
		if len(step.CallVars) > 0 {
			return Step{}, fmt.Errorf("use %q: with: is not supported here; use: shares the caller's scope, so there is nothing to pass in — use call: for an isolated task", step.CallTarget)
		}
		if len(step.IntoEntries) > 0 {
			return Step{}, fmt.Errorf("use %q: into: is not supported here; use: shares the caller's scope, so results are already there — use call: for an isolated task", step.CallTarget)
		}
	} else if step.Rerun {
		return Step{}, fmt.Errorf("rerun: is only supported on use: steps")
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

func parseLoopNode(fieldVal *yaml.Node) (forTarget string, forList []string, forMatrix []ForMatrixEntry, err error) {
	switch fieldVal.Kind {
	case yaml.SequenceNode:
		for _, item := range fieldVal.Content {
			forList = append(forList, item.Value)
		}
	case yaml.MappingNode:
		for _, mapEntry := range mapEntries(fieldVal.Content) {
			varName := mapEntry.Key
			if err := validateVarName(varName); err != nil {
				return "", nil, nil, err
			}
			valNode := mapEntry.Val
			entry := ForMatrixEntry{VarName: varName}
			if valNode.Kind == yaml.SequenceNode {
				for _, item := range valNode.Content {
					entry.List = append(entry.List, item.Value)
				}
			} else {
				entry.ListTmpl = normalizeTmpl(valNode.Value)
			}
			forMatrix = append(forMatrix, entry)
		}
	default:
		forTarget = normalizeTmpl(fieldVal.Value)
	}
	return forTarget, forList, forMatrix, nil
}
