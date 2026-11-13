package literal

import (
	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// Into is the value side of one into: entry: a single leaf, or a list/map
// literal of leaves. A leaf is a source head name with accessors and filters
// (stdout[0] | trim), or a {{ }} template over the caller's vars; see
// eval.EvalIntoLeaf.
type Into struct {
	leaf string
	tree *node
}

// ParseInto parses the value of the into: entry named name. A scalar is one
// leaf, stored as written; a mapping or sequence is a literal of leaves whose
// list elements may spread (stdout...). A spread anywhere else is an error
// wrapping ErrSpread. name roots the field path in error messages.
func ParseInto(name string, n *yaml.Node) (Into, error) {
	if n.Kind != yaml.MappingNode && n.Kind != yaml.SequenceNode {
		leaf, err := singleLeaf(intoLeaf, n.Value)
		if err != nil {
			return Into{}, err
		}
		return Into{leaf: leaf}, nil
	}
	tree, err := parseTree(n, intoLeaf, name)
	if err != nil {
		return Into{}, err
	}
	return Into{tree: &tree}, nil
}

// IntoLeaf is the into: value that reads name from the source as is: what a
// bare into: NAME, and each results: name a call: pulls, stands for.
func IntoLeaf(name string) Into {
	return Into{leaf: name}
}

// Eval evaluates i, each leaf resolving its head through src with vars, the
// caller's scope, underneath.
func (i Into) Eval(src eval.IntoSource, vars map[string]value.Value) (value.Value, error) {
	evalLeaf := func(leaf string) (value.Value, error) {
		return eval.EvalIntoLeaf(leaf, src, vars)
	}
	if i.tree == nil {
		return evalLeaf(i.leaf)
	}
	tree, err := i.tree.eval(evalLeaf)
	if err != nil {
		return value.Value{}, err
	}
	return value.Of(tree), nil
}
