package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
			forTarget, forList, forMatrix, err := parseLoopNode(fieldVal)
			if err != nil {
				return s, err
			}
			s.ForTarget, s.ForList, s.ForMatrix = forTarget, forList, forMatrix
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
			s.Soft = parseBool(fieldVal)
		case "interactive":
			v := parseBool(fieldVal)
			s.Interactive = &v
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

func parseLoopNode(fieldVal *yaml.Node) (forTarget string, forList []string, forMatrix []ForMatrixEntry, err error) {
	switch fieldVal.Kind {
	case yaml.SequenceNode:
		for _, item := range fieldVal.Content {
			forList = append(forList, item.Value)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(fieldVal.Content); i += 2 {
			varName := fieldVal.Content[i].Value
			if strings.Contains(varName, "{{") {
				return "", nil, nil, fmt.Errorf("variable name %q must not contain template syntax", varName)
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
			forMatrix = append(forMatrix, entry)
		}
	default:
		forTarget = normalizeTmpl(fieldVal.Value)
	}
	return forTarget, forList, forMatrix, nil
}
