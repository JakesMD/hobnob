package web

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"hobnob/internal/value"
)

// KindMenu is a prompt that offers options, one choice or several.
const KindMenu = "menu"

// PromptSelect asks for a choice among items in the page. Its signature is
// runner.SelectPromptFunc's; check: arrives as validate, which judges the
// answer as the terminal's menu does: the chosen item, or under multi the
// whole list. A secret menu is masked on the rail only.
func (s *state) PromptSelect(ctx context.Context, varName, info string, items []string,
	multi bool, defaultVal, _ string, secret, optional bool,
	validate func(value.Value) (bool, error), checkDesc string) (value.Value, error) {
	prompt := Prompt{Kind: KindMenu, VarName: varName, Info: info, Options: items, Multi: multi,
		Optional: optional, Secret: secret}
	defaults := menuDefaults(items, multi, defaultVal)
	if multi {
		prompt.Defaults = defaults
	} else if len(defaults) > 0 {
		prompt.Default, prompt.HasDefault = defaults[0], true
	}
	var answer value.Value
	skipped, err := s.ask(ctx, prompt, func(raw json.RawMessage) (RailEntry, string, bool) {
		picks, refusal := menuPicks(raw, items, multi, defaults)
		if refusal != "" {
			return RailEntry{}, refusal, false
		}
		if len(picks) == 0 && optional {
			return RailEntry{}, "", true
		}
		if len(picks) == 0 {
			if multi {
				return RailEntry{}, "pick at least one option", false
			}
			return RailEntry{}, "an answer is required", false
		}
		candidate := value.Str(picks[0])
		if multi {
			list := make([]any, len(picks))
			for i, pick := range picks {
				list[i] = pick
			}
			candidate = value.Of(list)
		}
		if validate != nil {
			ok, err := validate(candidate)
			if err != nil {
				return RailEntry{}, fmt.Sprintf("check failed: %v", err), false
			}
			if !ok {
				return RailEntry{}, "validation failed: " + checkDesc, false
			}
		}
		answer = candidate
		entry := RailEntry{Label: label(info, varName), Value: strings.Join(picks, ", ")}
		if secret {
			entry.Value = maskedAnswer
		}
		return entry, "", false
	})
	if err != nil {
		return value.Value{}, err
	}
	if skipped {
		if multi {
			return value.Of([]any{}), nil
		}
		return value.Str(""), nil
	}
	return answer, nil
}

// menuDefaults is the options a menu starts ticked or chosen: under multi a
// JSON list of options or one option, otherwise the one option defaultVal
// names. Anything that isn't an option is dropped.
func menuDefaults(items []string, multi bool, defaultVal string) []string {
	if defaultVal == "" {
		return nil
	}
	wanted := []string{defaultVal}
	if multi {
		if parsed, err := value.Parse(defaultVal); err == nil && parsed.Kind() == value.KindArray {
			wanted = nil
			for _, item := range parsed.Any().([]any) {
				wanted = append(wanted, value.Of(item).String())
			}
		}
	}
	var defaults []string
	for _, item := range items {
		if slices.Contains(wanted, item) {
			defaults = append(defaults, item)
		}
	}
	if !multi && len(defaults) > 1 {
		defaults = defaults[:1]
	}
	return defaults
}

// menuPicks reads an answer's choices in the options' order. null takes the
// defaults, as Continue on an untouched menu does.
func menuPicks(raw json.RawMessage, items []string, multi bool, defaults []string) ([]string, string) {
	if string(raw) == "null" {
		return defaults, ""
	}
	var chosen []string
	if multi {
		if err := json.Unmarshal(raw, &chosen); err != nil {
			return nil, "the answer must be a list of options"
		}
	} else {
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, "the answer must be one of the options"
		}
		if one != "" {
			chosen = []string{one}
		}
	}
	for _, pick := range chosen {
		if !slices.Contains(items, pick) {
			return nil, fmt.Sprintf("%q is not one of the options", pick)
		}
	}
	var picks []string
	for _, item := range items {
		if slices.Contains(chosen, item) {
			picks = append(picks, item)
		}
	}
	return picks, ""
}
