package glob

import "testing"

func TestText(t *testing.T) {
	// given patterns and a whole text, when matched, then the answer is kept
	// only if some plain pattern (or none) matches and no ! pattern does
	// (why: one match: meaning for menus and text answers)
	cases := []struct {
		name     string
		patterns []string
		text     string
		want     bool
	}{
		{"no patterns keeps everything", nil, "anything", true},
		{"star stops at slash", []string{"feature/*"}, "feature/a/b", false},
		{"star within one segment", []string{"feature/*"}, "feature/login", true},
		{"double star crosses slash", []string{"feature/**"}, "feature/a/b", true},
		{"case-insensitive", []string{"V1.*"}, "v1.2", true},
		{"question mark stops at slash", []string{"a?b"}, "a/b", false},
		{"question mark one char", []string{"a?b"}, "axb", true},
		{"class", []string{"[abc]x"}, "bx", true},
		{"class miss", []string{"[abc]x"}, "dx", false},
		{"alternation", []string{"*.{yml,yaml}"}, "a.YAML", true},
		{"any plain pattern is enough", []string{"*.go", "*.md"}, "r.md", true},
		{"no plain pattern matches", []string{"*.go", "*.md"}, "r.txt", false},
		{"negation alone keeps the rest", []string{"!.*"}, "visible", true},
		{"negation drops a match", []string{"!.*"}, ".hidden", false},
		{"negation beats plain", []string{"*.yml", "!ci.*"}, "ci.yml", false},
		{"order does not matter", []string{"!ci.*", "*.yml"}, "ci.yml", false},
		{"whole text only", []string{"v1"}, "v10", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			matcher, err := New(tc.patterns)
			if err != nil {
				t.Fatal(err)
			}

			// Act
			got := matcher.Text(tc.text)

			// Assert
			if got != tc.want {
				t.Errorf("Text(%q) with %v = %v, want %v", tc.text, tc.patterns, got, tc.want)
			}
		})
	}
}

func TestPath(t *testing.T) {
	// given a pattern form and an absolute path, when matched, then the form
	// decides what the pattern is compared against (why: the picker's §2
	// table)
	const work, home = "/work/proj", "/home/me"
	cases := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{"name form matches the base name", []string{"*.yml"}, "/x/y/a.yml", true},
		{"name form ignores directories", []string{"y"}, "/x/y/a.yml", false},
		{"name form is case-insensitive", []string{"README*"}, "/x/readme.md", true},
		{"slash inside matches the path end", []string{"lib/*"}, "/a/b/lib/x.go", true},
		{"slash inside at any depth", []string{"lib/*"}, "/lib/x.go", true},
		{"slash inside is anchored to a segment", []string{"lib/*"}, "/a/mylib/x.go", false},
		{"slash inside star stays in a segment", []string{"lib/*"}, "/a/lib/b/x.go", false},
		{"slash inside double star", []string{"lib/**"}, "/a/lib/b/x.go", true},
		{"leading slash is absolute", []string{"/work/*"}, "/work/a.txt", true},
		{"leading slash miss", []string{"/work/*"}, "/other/a.txt", false},
		{"tilde expands to home", []string{"~/docs/*"}, "/home/me/docs/a.txt", true},
		{"tilde miss", []string{"~/docs/*"}, "/home/you/docs/a.txt", false},
		{"dot slash is from the working directory", []string{"./configs/*"}, "/work/proj/configs/a.yml", true},
		{"dot slash is anchored", []string{"./configs/*"}, "/work/proj/x/configs/a.yml", false},
		{"dot slash outside the working directory", []string{"./configs/*"}, "/work/configs/a.yml", false},
		{"negated name", []string{"!.*"}, "/x/.git", false},
		{"negated name keeps others", []string{"!.*"}, "/x/src", true},
		{"negated path form", []string{"!/x/skip/*"}, "/x/skip/a", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			matcher, err := New(tc.patterns)
			if err != nil {
				t.Fatal(err)
			}

			// Act
			got := matcher.Path(tc.path, work, home)

			// Assert
			if got != tc.want {
				t.Errorf("Path(%q) with %v = %v, want %v", tc.path, tc.patterns, got, tc.want)
			}
		})
	}
}

func TestPathParts(t *testing.T) {
	// given plain and negated patterns, when asked separately, then Included
	// reports the plain side and Excluded the negated side (why: the picker
	// never hides directories for plain patterns but does for ! patterns)
	// Arrange
	matcher, err := New([]string{"*.yml", "!.*"})
	if err != nil {
		t.Fatal(err)
	}

	// Act
	includedDir := matcher.Included("/x/dir", "/w", "/h")
	excludedGit := matcher.Excluded("/x/.git", "/w", "/h")
	excludedDir := matcher.Excluded("/x/dir", "/w", "/h")

	// Assert
	if includedDir {
		t.Error("Included(dir) = true, want false (plain pattern misses)")
	}
	if !excludedGit {
		t.Error("Excluded(.git) = false, want true")
	}
	if excludedDir {
		t.Error("Excluded(dir) = true, want false")
	}
}

func TestNewRejectsBadPattern(t *testing.T) {
	// given a malformed pattern, when compiled, then New errors naming it
	// (why: a typo must fail loudly, not silently match nothing)
	// Arrange
	patterns := []string{"[abc"}

	// Act
	_, err := New(patterns)

	// Assert
	if err == nil {
		t.Fatal("New([abc) succeeded, want error")
	}
}
