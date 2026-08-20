package eval

import "text/template/parse"

// ReferencedVars returns the top-level scope-var names expr references —
// every .NAME, whether bare or as the head of an accessor/filter chain, at
// any nesting depth (including a dynamic key's own template, [.KEY]) — in
// first-seen order with duplicates removed. Used by const:'s closed-world
// check: an entry may only reference earlier const: keys and the builtins,
// so the check needs every name a template touches, not just its outermost
// token.
func ReferencedVars(expr string) ([]string, error) {
	tmpl, err := parseTemplateCached(expr)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var names []string
	var walk func(parse.Node)
	walk = func(n parse.Node) {
		if n == nil {
			return
		}
		switch node := n.(type) {
		case *parse.ListNode:
			for _, c := range node.Nodes {
				walk(c)
			}
		case *parse.ActionNode:
			walk(node.Pipe)
		case *parse.PipeNode:
			for _, cmd := range node.Cmds {
				walk(cmd)
			}
		case *parse.CommandNode:
			for _, arg := range node.Args {
				walk(arg)
			}
		case *parse.FieldNode:
			if len(node.Ident) > 0 {
				name := node.Ident[0]
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		case *parse.ChainNode:
			walk(node.Node)
		case *parse.IfNode:
			walk(node.Pipe)
			walk(node.List)
			walk(node.ElseList)
		case *parse.RangeNode:
			walk(node.Pipe)
			walk(node.List)
			walk(node.ElseList)
		case *parse.WithNode:
			walk(node.Pipe)
			walk(node.List)
			walk(node.ElseList)
		}
	}
	walk(tmpl.Tree.Root)
	return names, nil
}
