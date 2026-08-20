package eval

import (
	"fmt"
	"strconv"
	"strings"
)

// rewriteAccessors rewrites every accessor chain inside {{ }} actions —
// .A.b, .A[0], .A[.KEY], .A[1:3], .A[*], .A["quoted key"], (.A | json)[0]
// — into a call to hobnob's own hbpath function, leaving all other bytes,
// including text outside actions and the contents of string/rune/raw-string
// literals, byte-identical. See DESIGN-PATH.md.
//
// Deliberately does not track source positions through the rewrite: a
// malformed accessor is reported with an exact offset into the ORIGINAL
// source (the error case below), which covers the vast majority of mistakes
// made with this feature. A residual text/template parse error after a
// successful rewrite (unbalanced {{, bad quoting elsewhere) reports against
// the rewritten text instead — accepted because the bytes before the first
// rewritten term are identical, and text/template's own error already
// quotes the offending token. Do not retrofit a position map without
// re-reading DESIGN-PATH.md's "Position mapping" section first.
func rewriteAccessors(src string) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(src) {
		rel := strings.Index(src[i:], "{{")
		if rel < 0 {
			out.WriteString(src[i:])
			break
		}
		actionStart := i + rel
		out.WriteString(src[i:actionStart])

		end, err := findActionEnd(src, actionStart)
		if err != nil {
			return "", err
		}
		body := src[actionStart+2 : end-2]
		rewritten, err := rewriteActionBody(body)
		if err != nil {
			return "", fmt.Errorf("accessor at offset %d: %w", actionStart, err)
		}
		out.WriteString("{{")
		out.WriteString(rewritten)
		out.WriteString("}}")
		i = end
	}
	return out.String(), nil
}

// findActionEnd returns the index just past the "}}" that closes the action
// starting at src[actionStart:actionStart+2], skipping over string/rune/
// raw-string literals so a "}}" inside one (or a stray "[" for that matter)
// is never mistaken for the delimiter.
func findActionEnd(src string, actionStart int) (int, error) {
	i := actionStart + 2
	for i < len(src) {
		switch src[i] {
		case '"':
			j, err := skipStringLit(src, i)
			if err != nil {
				return 0, err
			}
			i = j
		case '`':
			j, err := skipRawStringLit(src, i)
			if err != nil {
				return 0, err
			}
			i = j
		case '\'':
			j, err := skipCharLit(src, i)
			if err != nil {
				return 0, err
			}
			i = j
		case '}':
			if i+1 < len(src) && src[i+1] == '}' {
				return i + 2, nil
			}
			i++
		default:
			i++
		}
	}
	return 0, fmt.Errorf(`unterminated action (missing "}}")`)
}

func skipStringLit(s string, i int) (int, error) {
	j := i + 1
	for j < len(s) {
		switch s[j] {
		case '\\':
			j += 2
		case '"':
			return j + 1, nil
		default:
			j++
		}
	}
	return 0, fmt.Errorf("unterminated string literal")
}

func skipRawStringLit(s string, i int) (int, error) {
	idx := strings.IndexByte(s[i+1:], '`')
	if idx < 0 {
		return 0, fmt.Errorf("unterminated raw string literal")
	}
	return i + 1 + idx + 1, nil
}

func skipCharLit(s string, i int) (int, error) {
	j := i + 1
	for j < len(s) {
		switch s[j] {
		case '\\':
			j += 2
		case '\'':
			return j + 1, nil
		default:
			j++
		}
	}
	return 0, fmt.Errorf("unterminated rune literal")
}

// rewriteActionBody rewrites accessor terms inside one {{ }} action's body
// (with the delimiters already stripped). A term may start at '.', '$', or
// '(' only when the immediately preceding byte is one of "{ ( | space tab
// newline -" (or the start of the body) — the rule that keeps a plain
// numeric literal like 1.5 from being read as a field access.
func rewriteActionBody(body string) (string, error) {
	var out strings.Builder
	i := 0
	isBoundary := true
	for i < len(body) {
		c := body[i]
		switch c {
		case '"':
			j, err := skipStringLit(body, i)
			if err != nil {
				return "", err
			}
			out.WriteString(body[i:j])
			i = j
			isBoundary = false
			continue
		case '`':
			j, err := skipRawStringLit(body, i)
			if err != nil {
				return "", err
			}
			out.WriteString(body[i:j])
			i = j
			isBoundary = false
			continue
		case '\'':
			j, err := skipCharLit(body, i)
			if err != nil {
				return "", err
			}
			out.WriteString(body[i:j])
			i = j
			isBoundary = false
			continue
		}

		if isBoundary && (c == '.' || c == '$' || c == '(') {
			consumed, rewritten, err := scanTerm(body, i)
			if err != nil {
				return "", err
			}
			if consumed > 0 {
				if rewritten != "" {
					out.WriteString(rewritten)
				} else {
					out.WriteString(body[i : i+consumed])
				}
				i += consumed
				isBoundary = false
				continue
			}
		}

		out.WriteByte(c)
		isBoundary = isTermBoundary(c)
		i++
	}
	return out.String(), nil
}

func isTermBoundary(c byte) bool {
	switch c {
	case '{', '(', '|', ' ', '\t', '\n', '-':
		return true
	}
	return false
}

// scanTerm attempts to parse one accessor term — a head plus zero or more
// steps — starting at body[start], where body[start] is '.', '$', or '('.
// consumed is 0 when body[start] does not actually begin a term (a numeric
// literal like ".5", a bare "$", or a "(" that isn't followed by a "["
// subscript once its matching ")" is found — an ordinary grouping/function
// paren). rewritten is the (hbpath ...) replacement, or "" when the term has
// zero steps (a bare .VAR/$var that must be emitted unchanged, since a
// top-level bare reference is never rewritten and a subscript's dynamic key
// may itself be one).
func scanTerm(body string, start int) (consumed int, rewritten string, err error) {
	i := start
	var rootArg string

	switch body[i] {
	case '.':
		j := i + 1
		if j >= len(body) || !isIdentStart(body[j]) {
			return 0, "", nil
		}
		j++
		for j < len(body) && isIdentCont(body[j]) {
			j++
		}
		rootArg = body[i:j]
		i = j
	case '$':
		j := i + 1
		if j >= len(body) || !isIdentStart(body[j]) {
			return 0, "", nil
		}
		j++
		for j < len(body) && isIdentCont(body[j]) {
			j++
		}
		rootArg = body[i:j]
		i = j
	case '(':
		end, perr := scanBalanced(body, i, '(', ')')
		if perr != nil {
			return 0, "", perr
		}
		if end >= len(body) || body[end] != '[' {
			return 0, "", nil
		}
		inner := body[i+1 : end-1]
		innerRewritten, rerr := rewriteActionBody(inner)
		if rerr != nil {
			return 0, "", rerr
		}
		rootArg = "(" + innerRewritten + ")"
		i = end
	default:
		return 0, "", nil
	}

	headEnd := i
	var steps []string
	for i < len(body) {
		if body[i] == '.' && i+1 < len(body) && isStepKeyStart(body[i+1]) {
			j := i + 2
			for j < len(body) && isStepKeyCont(body[j]) {
				j++
			}
			steps = append(steps, strconv.Quote(body[i+1:j]))
			i = j
			continue
		}
		if body[i] == '[' {
			end, berr := scanBalanced(body, i, '[', ']')
			if berr != nil {
				return 0, "", berr
			}
			content := body[i+1 : end-1]
			stepArg, serr := parseSubscript(content)
			if serr != nil {
				return 0, "", fmt.Errorf("subscript %q: %w", content, serr)
			}
			steps = append(steps, stepArg)
			i = end
			continue
		}
		break
	}

	if len(steps) == 0 {
		return headEnd - start, "", nil
	}

	src := body[start:i]
	var b strings.Builder
	b.WriteString("(hbpath ")
	b.WriteString(strconv.Quote(src))
	b.WriteByte(' ')
	b.WriteString(rootArg)
	for _, s := range steps {
		b.WriteByte(' ')
		b.WriteString(s)
	}
	b.WriteByte(')')
	return i - start, b.String(), nil
}

// parseSubscript interprets the content between one term step's "[" and
// "]": "*" (every element/value), a "lo:hi" slice, a quoted string key, a
// bare (optionally negative) integer index, or a dynamic key — another term,
// possibly with zero steps of its own (a bare .VAR / $var reference).
func parseSubscript(content string) (string, error) {
	if content == "*" {
		return "hbstar", nil
	}
	if colon := findTopLevelByte(content, ':'); colon >= 0 {
		loArg, err := parseSliceBound(content[:colon])
		if err != nil {
			return "", err
		}
		hiArg, err := parseSliceBound(content[colon+1:])
		if err != nil {
			return "", err
		}
		return "(hbslice " + loArg + " " + hiArg + ")", nil
	}
	if content == "" {
		return "", fmt.Errorf("empty subscript")
	}
	if content[0] == '"' {
		end, err := skipStringLit(content, 0)
		if err != nil {
			return "", err
		}
		if end != len(content) {
			return "", fmt.Errorf("invalid subscript %q", content)
		}
		return content, nil
	}
	if content[0] == '-' || (content[0] >= '0' && content[0] <= '9') {
		if !isIntLiteral(content) {
			return "", fmt.Errorf("invalid subscript %q", content)
		}
		return content, nil
	}
	if content[0] == '.' || content[0] == '$' || content[0] == '(' {
		consumed, rewritten, err := scanTerm(content, 0)
		if err != nil {
			return "", err
		}
		if consumed != len(content) {
			return "", fmt.Errorf("invalid subscript %q", content)
		}
		if rewritten != "" {
			return rewritten, nil
		}
		return content, nil
	}
	return "", fmt.Errorf("invalid subscript %q", content)
}

// parseSliceBound validates one side of a "lo:hi" slice: empty (an absent
// bound — emitted as the empty-string sentinel value.Slice/clampSlice treats
// as "unbounded on this side") or a bare integer literal.
func parseSliceBound(s string) (string, error) {
	if s == "" {
		return `""`, nil
	}
	if !isIntLiteral(s) {
		return "", fmt.Errorf("slice bound %q must be an integer", s)
	}
	return s, nil
}

func isIntLiteral(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// scanBalanced scans s starting at i, where s[i] == open, returning the
// index just past the matching close byte — tracking only open/close depth
// (any other bracket type is opaque to it) and skipping string/rune/
// raw-string literals, so a bracket or paren inside one is never mistaken
// for structural nesting.
func scanBalanced(s string, i int, open, close byte) (int, error) {
	depth := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			j, err := skipStringLit(s, i)
			if err != nil {
				return 0, err
			}
			i = j
		case '`':
			j, err := skipRawStringLit(s, i)
			if err != nil {
				return 0, err
			}
			i = j
		case '\'':
			j, err := skipCharLit(s, i)
			if err != nil {
				return 0, err
			}
			i = j
		case open:
			depth++
			i++
		case close:
			depth--
			i++
			if depth == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated %q", string(close))
}

// findTopLevelByte returns the index of the first occurrence of target in s
// that is not inside a "[...]", "(...)", or string/rune/raw-string literal,
// or -1 if there is none.
func findTopLevelByte(s string, target byte) int {
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '"':
			j, err := skipStringLit(s, i)
			if err != nil {
				return -1
			}
			i = j
			continue
		case '`':
			j, err := skipRawStringLit(s, i)
			if err != nil {
				return -1
			}
			i = j
			continue
		case '\'':
			j, err := skipCharLit(s, i)
			if err != nil {
				return -1
			}
			i = j
			continue
		case '[':
			end, err := scanBalanced(s, i, '[', ']')
			if err != nil {
				return -1
			}
			i = end
			continue
		case '(':
			end, err := scanBalanced(s, i, '(', ')')
			if err != nil {
				return -1
			}
			i = end
			continue
		}
		if c == target {
			return i
		}
		i++
	}
	return -1
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isStepKeyStart(c byte) bool { return isIdentStart(c) }

func isStepKeyCont(c byte) bool { return isIdentCont(c) || c == '-' }

// IsBareRef reports whether s is exactly one accessor term optionally
// followed by "| filter | ..." — the brace-free form config.normalizeTmpl
// wraps in {{ }}. Mirrors the old bareVarRef regex's head-charset
// restriction ([A-Z][A-Z0-9_]*) so relative paths (./infra), dotfiles
// (.git), and lowercase-leading text never match — only the step grammar
// (bracket subscripts, dotted keys) is new; a step's own key charset is
// wider ([A-Za-z_][A-Za-z0-9_-]*), since object keys are mixed-case.
func IsBareRef(s string) bool {
	bar := findTopLevelByte(s, '|')
	head := s
	tail := ""
	hasBar := bar >= 0
	if hasBar {
		head = s[:bar]
		tail = s[bar+1:]
	}
	head = strings.TrimRight(head, " \t\r\n")
	if !isUpperHeadTerm(head) {
		return false
	}
	if !hasBar {
		return true
	}
	if tail == "" {
		return false
	}
	return !strings.Contains(tail, "{")
}

func isUpperHeadTerm(head string) bool {
	if len(head) < 2 || head[0] != '.' {
		return false
	}
	j := 1
	if !(head[j] >= 'A' && head[j] <= 'Z') {
		return false
	}
	j++
	for j < len(head) {
		c := head[j]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			j++
			continue
		}
		break
	}
	if j == len(head) {
		return true
	}
	if head[j] != '.' && head[j] != '[' {
		return false
	}
	consumed, _, err := scanTerm(head, 0)
	return err == nil && consumed == len(head)
}

// SplitSourceAccessor splits a run: into: pipe source token into its bare
// name and any trailing accessor: "stdout[0].name" -> ("stdout",
// "[0].name"); "stdout" -> ("stdout", "").
func SplitSourceAccessor(src string) (name, accessor string) {
	i := 0
	for i < len(src) && src[i] != '.' && src[i] != '[' {
		i++
	}
	return src[:i], src[i:]
}
