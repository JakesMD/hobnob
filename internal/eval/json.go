package eval

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/theory/jsonpath"
)

func splitLinesJSON(raw string) (string, error) {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	jsonBytes, err := json.Marshal(lines)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// decodeJSON parses a JSON string into an interface{} tree, preserving
// numbers as json.Number so pluck/keys/values round-trip them exactly
// (no float64 precision loss, no unwanted ".0" suffix).
func decodeJSON(raw string) (interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// stringifyJSONValue renders a decoded JSON leaf as a hobnob scope string:
// strings pass through unquoted, numbers/bools use their plain text form,
// and nested objects/arrays re-marshal to compact JSON so results stay
// chainable (pluck | pluck, pluck | loop, ...).
func stringifyJSONValue(value interface{}) (string, error) {
	switch typedValue := value.(type) {
	case nil:
		return "", nil
	case string:
		return typedValue, nil
	case bool:
		return strconv.FormatBool(typedValue), nil
	case json.Number:
		return typedValue.String(), nil
	default:
		jsonBytes, err := json.Marshal(typedValue)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}
}

// jsonPathParser parses pluck's path argument as an RFC 9535 JSONPath query
// (https://www.rfc-editor.org/rfc/rfc9535.html). Reused across calls — safe
// for concurrent use, parsing is the expensive part.
var jsonPathParser = jsonpath.NewParser()

// normalizeJSONPath lets pluck callers write "profile.name" or "[2]" instead
// of the RFC-required "$.profile.name" / "$[2]" — pluck always addresses
// from the root, so the "$" is implied rather than typed out each time.
func normalizeJSONPath(path string) string {
	switch {
	case strings.HasPrefix(path, "$"):
		return path
	case strings.HasPrefix(path, "["):
		return "$" + path
	default:
		return "$." + path
	}
}

// sortedMapEntries decodes raw as a JSON object and returns its keys (sorted)
// alongside their stringified values, so iteration order is deterministic.
func sortedMapEntries(raw string) (keys []string, values []string, err error) {
	decoded, err := decodeJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	object, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("not a JSON object")
	}
	keys = make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values = make([]string, 0, len(keys))
	for _, key := range keys {
		stringValue, err := stringifyJSONValue(object[key])
		if err != nil {
			return nil, nil, err
		}
		values = append(values, stringValue)
	}
	return keys, values, nil
}

// IsJSONObject reports whether raw (trimmed) looks like a JSON object, used
// by execFor to pick list vs. map loop iteration.
func IsJSONObject(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "{")
}

// ParseMapEntries decodes a JSON object string into sorted keys and their
// stringified values, for loop: map iteration (.KEY / .VALUE per pass).
func ParseMapEntries(raw string) (keys []string, values []string, err error) {
	keys, values, err = sortedMapEntries(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid map %q: %w", raw, err)
	}
	return keys, values, nil
}

// ParseList parses a JSON array string or a single value into a []string.
// Returns nil for empty input.
func ParseList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, fmt.Errorf("invalid list %q: %w", raw, err)
		}
		return items, nil
	}
	return []string{raw}, nil
}
