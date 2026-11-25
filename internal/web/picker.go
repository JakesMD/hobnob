package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// KindPath is the prompt kind of a file or folder picker.
const KindPath = "path"

// Picker is a picker prompt's own part of the snapshot: the Finder-style
// column view down to the open folder.
type Picker struct {
	Type  string `json:"type"` // "file" or "dir"
	Multi bool   `json:"multi,omitempty"`
	Open  string `json:"open"`
	// OpenPickable says the open folder itself may be the answer of a folder
	// picker.
	OpenPickable bool     `json:"openPickable,omitempty"`
	Columns      []Column `json:"columns"`
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

// junkNames are never listed, as in the terminal picker.
var junkNames = []string{
	".DS_Store", ".Spotlight-V100", ".Trashes", ".fseventsd", ".TemporaryItems",
	"Icon\r", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN", "System Volume Information",
}

func isJunk(name string) bool {
	if strings.HasPrefix(name, "._") {
		return true
	}
	for _, junk := range junkNames {
		if strings.EqualFold(name, junk) {
			return true
		}
	}
	return false
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
	switch {
	case written == "":
		return filepath.Clean(base)
	case written == "~" || strings.HasPrefix(written, "~/"):
		return filepath.Join(r.home, strings.TrimPrefix(written, "~"))
	case filepath.IsAbs(written):
		return filepath.Clean(written)
	}
	return filepath.Join(base, written)
}

func (r pickerRules) isDirPicker() bool { return r.prompt.Type == "dir" }

// allows says abs may be picked: not Excluded, and Included.
func (r pickerRules) allows(abs string, isDir bool) bool {
	return (r.prompt.Excluded == nil || !r.prompt.Excluded(abs, isDir)) &&
		(r.prompt.Included == nil || r.prompt.Included(abs, isDir))
}

func (r pickerRules) kindFits(isDir bool) bool { return isDir == r.isDirPicker() }

// list reads dir, hiding junk, the wrong kind of file and what match:
// excludes. Folders come first, then by name ignoring case.
func (r pickerRules) list(dir string) []Entry {
	items, _ := os.ReadDir(dir)
	var dirs, files []Entry
	for _, item := range items {
		if isJunk(item.Name()) {
			continue
		}
		abs := filepath.Join(dir, item.Name())
		isDir := item.IsDir()
		if item.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(abs); err == nil {
				isDir = info.IsDir()
			}
		}
		if r.isDirPicker() && !isDir {
			continue
		}
		if r.prompt.Excluded != nil && r.prompt.Excluded(abs, isDir) {
			continue
		}
		included := r.prompt.Included == nil || r.prompt.Included(abs, isDir)
		switch {
		case isDir:
			dirs = append(dirs, Entry{Name: item.Name(), Dir: true, Pickable: r.isDirPicker() && included})
		case included:
			files = append(files, Entry{Name: item.Name(), Pickable: true})
		}
	}
	byName := func(entries []Entry) {
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
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
	view := &Picker{Type: r.prompt.Type, Multi: r.prompt.Multi, Open: open, Selected: selected,
		OpenPickable: r.isDirPicker() && r.allows(open, true), Columns: []Column{}}
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

// start is where the picker opens and what it starts with picked. A default
// of the picker's kind is picked and its folder shown (a folder picker opens
// the default folder itself); a folder given to a file picker is where it
// starts; anything else opens the nearest folder that exists.
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
	open = nearestFolder(open)
	if r.isDirPicker() && !prompt.Multi && r.allows(open, true) {
		selected = []string{open}
	}
	return open, selected
}

func nearestFolder(dir string) string {
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return dir
		}
		dir = next
	}
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
		Refusal: prompt.Error, Picker: rules.view(open, selected)}
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
