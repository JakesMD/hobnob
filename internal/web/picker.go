package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// KindPath is the prompt kind of a file or folder picker.
const KindPath = "path"

// Picker is a picker prompt's own part of the snapshot: the Finder-style
// column view down to the open folder.
type Picker struct {
	Type    string   `json:"type"` // "file" or "dir"
	Open    string   `json:"open"`
	Columns []Column `json:"columns"`
	// Selected is what the picker starts with picked; the page tracks the
	// picks after that.
	Selected []string `json:"selected"`
}

// Column lists one folder, already filtered. Next names the entry that the
// following column is the listing of; it is empty in the last column.
type Column struct {
	Path    string  `json:"path"`
	Next    string  `json:"next,omitempty"`
	Entries []Entry `json:"entries"`
}

// Entry is a file or folder that is listed. A folder that isn't Pickable can
// still be opened.
type Entry struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir,omitempty"`
	Pickable bool   `json:"pickable,omitempty"`
}

// pickerRules is what a picker prompt allows, from its tui.PathPrompt.
type pickerRules struct {
	prompt tui.PathPrompt
	home   string
}

func newPickerRules(prompt tui.PathPrompt) pickerRules {
	home, _ := os.UserHomeDir()
	if prompt.WorkDir == "" {
		prompt.WorkDir = prompt.Invoke
	}
	return pickerRules{prompt: prompt, home: home}
}

func (r pickerRules) resolve(written, base string) string {
	return tui.ResolvePath(written, base, r.home)
}

func (r pickerRules) isDirPicker() bool { return r.prompt.Type == "dir" }

// allows says abs may be picked, by the terminal picker's rules.
func (r pickerRules) allows(abs string, isDir bool) bool {
	return r.prompt.Allows(abs, isDir)
}

func (r pickerRules) kindFits(isDir bool) bool { return isDir == r.isDirPicker() }

func (r pickerRules) list(dir string) []Entry {
	listed := tui.ListDir(dir, r.prompt)
	entries := make([]Entry, len(listed))
	for i, e := range listed {
		entries[i] = Entry{Name: e.Name, Dir: e.Dir, Pickable: e.Pickable}
	}
	return entries
}

// view lists the columns from the home folder, or the filesystem root when
// open isn't under it, down to open.
func (r pickerRules) view(open string, selected []string) *Picker {
	root := string(filepath.Separator)
	if r.home != "" && (open == r.home || strings.HasPrefix(open, r.home+string(filepath.Separator))) {
		root = r.home
	}
	chain := []string{open}
	for dir := open; dir != root; {
		dir = filepath.Dir(dir)
		chain = append([]string{dir}, chain...)
	}
	view := &Picker{Type: r.prompt.Type, Open: open, Selected: selected, Columns: []Column{}}
	for i, dir := range chain {
		column := Column{Path: dir, Entries: r.list(dir)}
		if i+1 < len(chain) {
			column.Next = filepath.Base(chain[i+1])
		}
		view.Columns = append(view.Columns, column)
	}
	if view.Selected == nil {
		view.Selected = []string{}
	}
	return view
}

// start is where the picker opens and what it starts with picked. See
// docs/guide/browser-mode.md for the rules.
func (r pickerRules) start() (open string, selected []string) {
	prompt := r.prompt
	open = r.resolve("", prompt.WorkDir)
	if prompt.Start != "" {
		open = r.resolve(prompt.Start, prompt.Invoke)
	}
	for i, written := range prompt.Default {
		if i > 0 && !prompt.Multi {
			break
		}
		abs := r.resolve(written, prompt.WorkDir)
		info, err := os.Stat(abs)
		if err != nil || !r.kindFits(info.IsDir()) || !r.allows(abs, info.IsDir()) {
			continue
		}
		selected = append(selected, abs)
	}
	if len(prompt.Default) > 0 {
		abs := r.resolve(prompt.Default[0], prompt.WorkDir)
		open = filepath.Dir(abs)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			open = abs
		}
	}
	return tui.NearestFolder(open), selected
}

// pick checks the answer's paths, whatever the page sent, and says why not.
func (r pickerRules) pick(raw json.RawMessage) (paths []string, refusal string) {
	if r.prompt.Multi {
		if err := json.Unmarshal(raw, &paths); err != nil {
			return nil, "the answer must be a list of paths"
		}
	} else {
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, "the answer must be a path"
		}
		if one != "" {
			paths = []string{one}
		}
	}
	seen := map[string]bool{}
	kept := paths[:0]
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, fmt.Sprintf("%s isn't an absolute path", path)
		}
		path = filepath.Clean(path)
		info, err := os.Stat(path)
		switch {
		case err != nil:
			return nil, fmt.Sprintf("%s doesn't exist", path)
		case r.isDirPicker() && !info.IsDir():
			return nil, fmt.Sprintf("%s isn't a folder", path)
		case !r.isDirPicker() && info.IsDir():
			return nil, fmt.Sprintf("%s is a folder, not a file", path)
		case !r.allows(path, info.IsDir()):
			return nil, fmt.Sprintf("%s doesn't match", path)
		}
		if !seen[path] {
			seen[path] = true
			kept = append(kept, path)
		}
	}
	return kept, ""
}

// PromptPath asks for a file or folder in the page. Its signature is
// runner.PathPromptFunc's. The picker the page shows is the one the terminal
// gets: the same start, the same predicates for match:.
func (s *state) PromptPath(ctx context.Context, prompt tui.PathPrompt, _ string, secret bool) (value.Value, bool, error) {
	rules := newPickerRules(prompt)
	open, selected := rules.start()
	p := Prompt{Kind: KindPath, VarName: prompt.VarName, Info: prompt.Info, Optional: prompt.Optional,
		Multi: prompt.Multi, Refusal: prompt.Error, Picker: rules.view(open, selected)}
	var picked []string
	skipped, err := s.askPicker(ctx, p, rules, func(raw json.RawMessage) (RailEntry, string, bool) {
		paths, refusal := rules.pick(raw)
		if refusal != "" {
			return RailEntry{}, refusal, false
		}
		if len(paths) == 0 {
			if prompt.Optional {
				return RailEntry{}, "", true
			}
			return RailEntry{}, "an answer is required", false
		}
		picked = paths
		entry := RailEntry{Label: label(prompt.Info, prompt.VarName), Value: strings.Join(paths, ", ")}
		if secret {
			entry.Value = maskedAnswer
		}
		return entry, "", false
	})
	if err != nil {
		return value.Value{}, false, err
	}
	if skipped {
		return value.Value{}, false, nil
	}
	if prompt.Multi {
		list := make([]any, len(picked))
		for i, path := range picked {
			list[i] = path
		}
		return value.Of(list), true, nil
	}
	return value.Str(picked[0]), true, nil
}

// askPicker is ask for a picker, which can also change its open folder.
func (s *state) askPicker(ctx context.Context, p Prompt, rules pickerRules, judge judge) (bool, error) {
	selected := p.Picker.Selected
	return s.ask(ctx, p, judge, func(open *openPrompt) {
		open.browse = func(path string) error {
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() || !filepath.IsAbs(path) {
				return fmt.Errorf("%s isn't a folder to open", path)
			}
			view := rules.view(filepath.Clean(path), selected)
			return s.settle(open, func() { open.Picker = view })
		}
	})
}

type openRequest struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

// openFolder is the POST /actions/open handler: a picker's open folder
// changes, and the next snapshot lists it.
func (s *state) openFolder(body json.RawMessage) error {
	var req openRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	open, err := s.openPrompt(req.ID)
	if err != nil {
		return err
	}
	if open.browse == nil {
		return fmt.Errorf("%s has no folder to open", open.VarName)
	}
	return open.browse(req.Path)
}
