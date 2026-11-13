package literal

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

type nodeKind int

const (
	// leafNode is a string leaf, evaluated at run time by its literal's leaf
	// evaluator: a template for Expr, a source expression for Into.
	leafNode nodeKind = iota
	// scalarNode is a non-string YAML scalar (bool/int/float/null), decoded
	// at load and passed through: a {{ }} action only occurs in a string.
	scalarNode
	objectNode
	arrayNode
)

// node is one node of a list or map literal. Evaluating it builds a real Go
// tree, never JSON text, so a leaf holding a quote or backslash cannot
// corrupt the structure around it.
type node struct {
	kind   nodeKind
	text   string // leafNode: the stored leaf
	spread string // leafNode: the reference before a trailing ..., whose items join the list; "" if not a spread
	scalar any    // scalarNode
	fields []field
	elems  []node
}

type field struct {
	key  string
	node node
}

// parseTree parses a YAML mapping, sequence or scalar into a node tree,
// storing each string leaf as grammar returns it. path is the dotted field
// path to n (CUSTOM.profile.name), used only to prefix error messages.
func parseTree(n *yaml.Node, grammar leafGrammar, path string) (node, error) {
	if n.Kind == yaml.AliasNode {
		return parseTree(n.Alias, grammar, path)
	}
	switch n.Kind {
	case yaml.MappingNode:
		fields := make([]field, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i].Value, n.Content[i+1]
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if strings.Contains(key, "{{") {
				return node{}, fmt.Errorf("%s: variable name %q must not contain template syntax", childPath, key)
			}
			child, err := parseTree(val, grammar, childPath)
			if err != nil {
				return node{}, err
			}
			if child.spread != "" {
				return node{}, fmt.Errorf("%s: %w", childPath, spreadError{raw: val.Value})
			}
			fields = append(fields, field{key: key, node: child})
		}
		return node{kind: objectNode, fields: fields}, nil
	case yaml.SequenceNode:
		elems := make([]node, len(n.Content))
		for i, item := range n.Content {
			child, err := parseTree(item, grammar, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return node{}, err
			}
			elems[i] = child
		}
		return node{kind: arrayNode, elems: elems}, nil
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			text, spread := grammar(n.Value)
			return node{kind: leafNode, text: text, spread: spread}, nil
		}
		var scalar any
		if err := n.Decode(&scalar); err != nil {
			return node{}, fmt.Errorf("%s: failed to decode literal %q: %w", path, n.Value, err)
		}
		return node{kind: scalarNode, scalar: value.Canonical(scalar)}, nil
	default:
		return node{}, fmt.Errorf("%s: unsupported YAML node in JSON literal", path)
	}
}

// eval assembles n into a Go tree, evaluating each leaf with evalLeaf and
// splicing a spread leaf's items into its list.
func (n node) eval(evalLeaf func(string) (value.Value, error)) (any, error) {
	switch n.kind {
	case objectNode:
		obj := make(map[string]any, len(n.fields))
		for _, f := range n.fields {
			val, err := f.node.eval(evalLeaf)
			if err != nil {
				return nil, err
			}
			obj[f.key] = val
		}
		return obj, nil
	case arrayNode:
		arr := make([]any, 0, len(n.elems))
		for _, elem := range n.elems {
			if elem.spread != "" {
				val, err := evalLeaf(elem.text)
				if err != nil {
					return nil, err
				}
				items, err := spreadItems(elem.spread, val)
				if err != nil {
					return nil, err
				}
				arr = append(arr, items...)
				continue
			}
			val, err := elem.eval(evalLeaf)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		return arr, nil
	case scalarNode:
		return n.scalar, nil
	default: // leafNode
		val, err := evalLeaf(n.text)
		if err != nil {
			return nil, err
		}
		return val.Any(), nil
	}
}

// refs returns every variable name n's template leaves read, spreads
// included.
func (n node) refs() ([]string, error) {
	switch n.kind {
	case leafNode:
		return eval.ReferencedVars(n.text)
	case objectNode, arrayNode:
		children := make([]node, 0, len(n.fields)+len(n.elems))
		for _, f := range n.fields {
			children = append(children, f.node)
		}
		children = append(children, n.elems...)
		var out []string
		for _, child := range children {
			refs, err := child.refs()
			if err != nil {
				return nil, err
			}
			out = append(out, refs...)
		}
		return out, nil
	default:
		return nil, nil
	}
}
