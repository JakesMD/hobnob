package eval

import "testing"

func TestCutSpread(t *testing.T) {
	tests := []struct {
		name    string
		elem    string
		wantRef string
		wantOK  bool
	}{
		{"given a bare reference with a trailing ..., when cut, then it is a spread of the reference (why: [.A..., d] splices A in)", ".A...", ".A", true},
		{"given an accessor chain with a trailing ..., when cut, then the whole chain is spread (why: spread applies to the whole element reference)", ".QUESTIONS[*].category...", ".QUESTIONS[*].category", true},
		{"given a parenthesised pipeline with a trailing ..., when cut, then the pipeline result is spread (why: (.X | json)... spreads parsed text)", "(.X | json)...", "(.X | json)", true},
		{"given a filter chain with a trailing ..., when cut, then the filtered result is spread (why: filters are part of the element reference)", ".X | json...", ".X | json", true},
		{"given a bare reference with no ..., when cut, then it is not a spread (why: without ... an element nests)", ".A", "", false},
		{"given plain text ending in ..., when cut, then it is not a spread (why: text like Loading... must stay text)", "Loading...", "", false},
		{"given a template action followed by ..., when cut, then it is not a spread (why: a template is how text really ending in ... is written)", "{{ .A }}...", "", false},
		{"given only ..., when cut, then it is not a spread (why: there is no reference to splice)", "...", "", false},
		{"given a relative path ending in ..., when cut, then it is not a spread (why: lowercase-leading text never matches a reference)", "./dir...", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			ref, ok := CutSpread(tc.elem)

			// Assert
			if ok != tc.wantOK || ref != tc.wantRef {
				t.Fatalf("CutSpread(%q) = (%q, %v), want (%q, %v)", tc.elem, ref, ok, tc.wantRef, tc.wantOK)
			}
		})
	}
}

func TestCutIntoSpread(t *testing.T) {
	tests := []struct {
		name     string
		leaf     string
		wantLeaf string
		wantOK   bool
	}{
		{"given a head name with a trailing ..., when cut, then it is a spread of the head (why: into: leaves name a source, not a template)", "stdout...", "stdout", true},
		{"given a dotted head with accessors and a trailing ..., when cut, then the whole leaf is spread (why: spread applies to the whole element reference)", ".NAMES[*].first...", ".NAMES[*].first", true},
		{"given a head with a filter and a trailing ..., when cut, then the filtered leaf is spread (why: filters are part of the element reference)", "stdout | json...", "stdout | json", true},
		{"given a head name with no ..., when cut, then it is not a spread (why: without ... an element nests)", "stdout", "", false},
		{"given a template leaf followed by ..., when cut, then it is not a spread (why: a {{ }} leaf is text)", "{{ .A }}...", "", false},
		{"given only ..., when cut, then it is not a spread (why: there is no head to splice)", "...", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			leaf, ok := CutIntoSpread(tc.leaf)

			// Assert
			if ok != tc.wantOK || leaf != tc.wantLeaf {
				t.Fatalf("CutIntoSpread(%q) = (%q, %v), want (%q, %v)", tc.leaf, leaf, ok, tc.wantLeaf, tc.wantOK)
			}
		})
	}
}
