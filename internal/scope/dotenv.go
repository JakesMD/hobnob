package scope

import (
	"fmt"
	"strings"
)

// parseDotenv reads KEY=VALUE text the way `source` reads a typical .env,
// without running it. Blank lines, # comments and an export prefix are
// skipped. An unquoted value drops a whitespace-led inline comment. Single
// quotes are literal. Double quotes and unquoted values expand $NAME,
// ${NAME}, ${NAME:-default} and ${NAME-default} (see expandDotenv). A quoted
// value may span lines.
//
// lookup resolves a referenced name; assign is called once per entry, in
// file order, so a later line can reference an earlier one through lookup.
// Anything that would otherwise lose text silently is an error naming the
// line: no =, a key that isn't a variable name, an unclosed quote, or text
// after a closing quote other than a comment.
func parseDotenv(data string, lookup func(string) (string, bool), assign func(key, val string)) error {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		line := strings.TrimSpace(lines[i])
		if line == "" || line[0] == '#' {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimLeft(rest, " \t")
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: expected KEY=VALUE, got %q", lineNo, line)
		}
		key = strings.TrimSpace(key)
		if key == "" || nameLen(key) != len(key) {
			return fmt.Errorf("line %d: %q is not a valid variable name", lineNo, key)
		}

		trimmed := strings.TrimLeft(raw, " \t")
		if trimmed == "" || (trimmed[0] != '"' && trimmed[0] != '\'') {
			assign(key, expandDotenv(stripInlineComment(raw), false, lookup))
			continue
		}

		quote := trimmed[0]
		body := trimmed[1:]
		var after string
		for {
			if end := closingQuote(body, quote); end >= 0 {
				body, after = body[:end], body[end+1:]
				break
			}
			i++
			if i >= len(lines) {
				return fmt.Errorf("line %d: %s: unterminated %c quote", lineNo, key, quote)
			}
			body += "\n" + lines[i]
		}
		if after = strings.TrimLeft(after, " \t"); after != "" && after[0] != '#' {
			return fmt.Errorf("line %d: %s: unexpected %q after closing %c quote", lineNo, key, after, quote)
		}
		if quote == '\'' {
			assign(key, body)
		} else {
			assign(key, expandDotenv(body, true, lookup))
		}
	}
	return nil
}

// closingQuote returns the index of the quote that closes s, or -1. Inside
// double quotes a backslash escapes the next byte; single quotes have no
// escapes.
func closingQuote(s string, quote byte) int {
	for i := 0; i < len(s); i++ {
		switch {
		case quote == '"' && s[i] == '\\':
			i++
		case s[i] == quote:
			return i
		}
	}
	return -1
}

// stripInlineComment cuts an unquoted value (as written after the =) at the
// first # that follows whitespace, then trims it. A # inside a word, or
// straight after the =, stays.
func stripInlineComment(s string) string {
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}

// expandDotenv substitutes references in s. In double quotes \n and \r are a
// newline and carriage return (as dotenv libraries read them) and a backslash
// escapes $, ", \ and `; unquoted, only \$ is an escape, so a Windows-style
// path keeps its backslashes. Any other backslash is kept.
func expandDotenv(s string, doubleQuoted bool, lookup func(string) (string, bool)) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			if escaped, ok := dotenvEscape(s[i+1], doubleQuoted); ok {
				out.WriteByte(escaped)
				i++
				continue
			}
		}
		if c != '$' || i+1 == len(s) {
			out.WriteByte(c)
			continue
		}

		if s[i+1] == '{' {
			end := matchingBrace(s[i+2:])
			if end < 0 {
				out.WriteByte(c)
				continue
			}
			out.WriteString(expandBraced(s[i+2:i+2+end], doubleQuoted, lookup))
			i += 2 + end
			continue
		}

		n := nameLen(s[i+1:])
		if n == 0 {
			out.WriteByte(c)
			continue
		}
		val, _ := lookup(s[i+1 : i+1+n])
		out.WriteString(val)
		i += n
	}
	return out.String()
}

// dotenvEscape reports what a backslash followed by next stands for, if it
// is an escape at all in the given quoting.
func dotenvEscape(next byte, doubleQuoted bool) (byte, bool) {
	switch {
	case next == '$':
		return '$', true
	case !doubleQuoted:
		return 0, false
	case next == 'n':
		return '\n', true
	case next == 'r':
		return '\r', true
	case next == '"' || next == '\\' || next == '`':
		return next, true
	}
	return 0, false
}

// matchingBrace returns the index of the } that closes a ${ whose body
// starts s, counting nested braces so ${B:-${A}} closes at the outer one, or
// -1 when it never closes.
func matchingBrace(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// expandBraced resolves the inside of ${...}: NAME, NAME:-default (default
// when unset or empty) or NAME-default (default only when unset). Anything
// else is left as written.
func expandBraced(inner string, doubleQuoted bool, lookup func(string) (string, bool)) string {
	asWritten := "${" + inner + "}"
	n := nameLen(inner)
	if n == 0 {
		return asWritten
	}
	val, set := lookup(inner[:n])
	switch rest := inner[n:]; {
	case rest == "":
		return val
	case strings.HasPrefix(rest, ":-"):
		if set && val != "" {
			return val
		}
		return expandDotenv(rest[2:], doubleQuoted, lookup)
	case strings.HasPrefix(rest, "-"):
		if set {
			return val
		}
		return expandDotenv(rest[1:], doubleQuoted, lookup)
	}
	return asWritten
}

// nameLen returns the length of the shell variable name s starts with
// ([A-Za-z_][A-Za-z0-9_]*), or 0.
func nameLen(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return i
	}
	return len(s)
}
