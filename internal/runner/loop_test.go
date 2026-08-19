package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"hobnob/internal/config"
	"hobnob/internal/value"
)

func makeForMatrixCfg(matrix []config.ForMatrixEntry, innerTmpl string) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{
				Kind:      config.KindFor,
				ForMatrix: matrix,
				ForSteps: []config.Step{{
					Kind:       config.KindSet,
					SetEntries: []config.SetEntry{{Key: "RESULT", ValTmpl: innerTmpl}},
				}},
			}}},
		},
	}
}

func TestExecFor_Matrix(t *testing.T) {
	tests := []struct {
		name       string
		matrix     []config.ForMatrixEntry
		innerTmpl  string
		initVars   map[string]value.Value
		wantResult string
	}{
		{
			name: "given single-var map form, when executed, then iterates using named variable (why: map form names iterator without as:)",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos", "windows"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: " ubuntu macos windows",
		},
		{
			name: "given two-var matrix, when executed, then runs full cartesian product",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}/{{.DB}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: " ubuntu/postgres ubuntu/sqlite macos/postgres macos/sqlite",
		},
		{
			name: "given two-var matrix with reversed key order, when executed, then second key becomes inner loop (why: first key is outermost per spec)",
			matrix: []config.ForMatrixEntry{
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
			},
			innerTmpl:  "{{.RESULT}} {{.DB}}/{{.PLATFORM}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: " postgres/ubuntu postgres/macos sqlite/ubuntu sqlite/macos",
		},
		{
			name: "given three-var matrix, when executed, then runs all combinations in correct nesting order",
			matrix: []config.ForMatrixEntry{
				{VarName: "OS", List: []string{"linux", "macos"}},
				{VarName: "ARCH", List: []string{"amd64", "arm64"}},
				{VarName: "DB", List: []string{"pg"}},
			},
			innerTmpl:  "{{.RESULT}} {{.OS}}/{{.ARCH}}/{{.DB}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: " linux/amd64/pg linux/arm64/pg macos/amd64/pg macos/arm64/pg",
		},
		{
			name: "given single-var matrix with one item, when executed, then runs exactly once",
			matrix: []config.ForMatrixEntry{
				{VarName: "ENV", List: []string{"prod"}},
			},
			innerTmpl:  "{{.RESULT}}{{.ENV}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: "prod",
		},
		{
			name: "given matrix with dynamic list template resolving to a real Array, when executed, then resolves list from vars (why: a captured/set: array stays typed all the way into the matrix loop — no re-parsing from text)",
			matrix: []config.ForMatrixEntry{
				{VarName: "NODE", ListTmpl: `{{.SERVERS}}`},
			},
			innerTmpl: "{{.RESULT}} {{.NODE}}",
			initVars: map[string]value.Value{
				"RESULT":  value.Str(""),
				"SERVERS": value.Of([]any{"web1", "web2"}),
			},
			wantResult: " web1 web2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := makeForMatrixCfg(test.matrix, test.innerTmpl)
			vars := copyVars(test.initVars)

			// Act
			err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := vars["RESULT"].String(); got != test.wantResult {
				t.Errorf("RESULT: got %q, want %q", got, test.wantResult)
			}
		})
	}
}

func makeForStringCfg(forList []string, forTarget, innerTmpl string) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{
				Kind:      config.KindFor,
				ForList:   forList,
				ForTarget: forTarget,
				ForSteps: []config.Step{{
					Kind:       config.KindSet,
					SetEntries: []config.SetEntry{{Key: "RESULT", ValTmpl: innerTmpl}},
				}},
			}}},
		},
	}
}

func TestExecFor_String(t *testing.T) {
	tests := []struct {
		name       string
		forList    []string
		forTarget  string
		innerTmpl  string
		initVars   map[string]value.Value
		wantResult string
	}{
		{
			name:       "given literal list, when executed, then iterates binding each item to ITEM (why: string form uses ITEM as default iterator)",
			forList:    []string{"alpha", "beta", "gamma"},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   sv(map[string]string{"RESULT": ""}),
			wantResult: " alpha beta gamma",
		},
		{
			name:      "given template target resolving to a real Array, when executed, then iterates its typed elements (why: a captured/set: array stays typed all the way into the loop — no re-parsing from text)",
			forTarget: "{{.FILES}}",
			innerTmpl: "{{.RESULT}} {{.ITEM}}",
			initVars: map[string]value.Value{
				"RESULT": value.Str(""),
				"FILES":  value.Of([]any{"x", "y", "z"}),
			},
			wantResult: " x y z",
		},
		{
			name:       "given empty list, when executed, then runs zero iterations (why: empty source must not error)",
			forList:    []string{},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   sv(map[string]string{"RESULT": "initial"}),
			wantResult: "initial",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := makeForStringCfg(test.forList, test.forTarget, test.innerTmpl)
			vars := copyVars(test.initVars)

			// Act
			err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := vars["RESULT"].String(); got != test.wantResult {
				t.Errorf("RESULT: got %q, want %q", got, test.wantResult)
			}
		})
	}
}

func TestExecFor_Map(t *testing.T) {
	// given loop: target resolves to a real Object, when executed, then
	// iterates sorted keys binding KEY/VALUE (why: map iteration is the
	// object counterpart to list iteration's ITEM — the object must already
	// be typed, e.g. from a set: map literal or a run: into: capture, never
	// sniffed from a plain string)
	// Arrange
	cfg := makeForStringCfg(nil, "{{.REGIONS}}", "{{.RESULT}} {{.KEY}}={{.VALUE}}")
	vars := copyVars(map[string]value.Value{
		"RESULT":  value.Str(""),
		"REGIONS": value.Of(map[string]any{"us": "us-east-1", "eu": "eu-west-1"}),
	})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := " eu=eu-west-1 us=us-east-1"
	if got := vars["RESULT"].String(); got != want {
		t.Errorf("RESULT: got %q, want %q", got, want)
	}
}

func TestExecFor_String_AlmostJSONObjectStaysString(t *testing.T) {
	// given loop: target is a plain string starting with { that isn't valid
	// JSON, when executed, then it's treated as a single string item — not
	// sniffed as a map and not an error (why: structure is only ever sniffed
	// once, at capture; loop: must not re-sniff a var that merely looks
	// JSON-shaped, which is what used to make this case error instead of
	// iterating once)
	// Arrange
	cfg := makeForStringCfg(nil, "{{.BAD}}", "{{.RESULT}}{{.ITEM}}")
	vars := copyVars(sv(map[string]string{"RESULT": "", "BAD": `{not valid json`}))

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "{not valid json"
	if got := vars["RESULT"].String(); got != want {
		t.Errorf("RESULT: got %q, want %q", got, want)
	}
}

func TestExecFor_Map_IteratorVarsRemovedAfterLoop(t *testing.T) {
	// given KEY/VALUE not in scope before loop, when map loop completes, then both removed from scope (why: map iterators must not leak into post-loop scope, mirrors ITEM behavior)
	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:      config.KindFor,
					ForTarget: "{{.MAP}}",
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.RESULT}}{{.KEY}}"},
						}},
					},
				},
			}},
		},
	}
	vars := map[string]value.Value{
		"RESULT": value.Str(""),
		"MAP":    value.Of(map[string]any{"a": "1"}),
	}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := vars["KEY"]; exists {
		t.Errorf("KEY should not exist after loop, got %q", vars["KEY"].String())
	}
	if _, exists := vars["VALUE"]; exists {
		t.Errorf("VALUE should not exist after loop, got %q", vars["VALUE"].String())
	}
}

func TestExecFor_IteratorVarRemovedAfterLoop(t *testing.T) {
	// given ITEM not in scope before loop, when for loop completes, then ITEM removed from scope (why: iterator must not leak into post-loop scope)
	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"a", "b", "c"},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.RESULT}}{{.ITEM}}"},
						}},
					},
				},
			}},
		},
	}
	vars := sv(map[string]string{"RESULT": ""})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := vars["RESULT"].String(); got != "abc" {
		t.Errorf("RESULT: got %q, want abc", got)
	}
	if _, exists := vars["ITEM"]; exists {
		t.Errorf("ITEM should not exist after loop, got %q", vars["ITEM"].String())
	}
}

func TestExecFor_IteratorVarRestoredIfPreexisting(t *testing.T) {
	// given ITEM exists before loop, when loop ends, then ITEM restored to prior value
	// (why: loop must not clobber caller's variable with the same name)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"x"},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.ITEM}}"},
						}},
					},
				},
			}},
		},
	}
	vars := sv(map[string]string{"ITEM": "original", "RESULT": ""})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := vars["ITEM"].String(); got != "original" {
		t.Errorf("ITEM: got %q, want original (should be restored)", got)
	}
}

func TestExecForMatrix_IteratorVarsRemovedAfterLoop(t *testing.T) {
	// given matrix iterator vars not in scope before loop, when matrix loop completes, then OS and ARCH removed from scope (why: matrix iterators must not leak into post-loop scope)
	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind: config.KindFor,
					ForMatrix: []config.ForMatrixEntry{
						{VarName: "OS", List: []string{"linux", "mac"}},
						{VarName: "ARCH", List: []string{"amd64"}},
					},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.RESULT}} {{.OS}}/{{.ARCH}}"},
						}},
					},
				},
			}},
		},
	}
	vars := sv(map[string]string{"RESULT": ""})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := vars["OS"]; exists {
		t.Errorf("OS should not exist after matrix loop, got %q", vars["OS"].String())
	}
	if _, exists := vars["ARCH"]; exists {
		t.Errorf("ARCH should not exist after matrix loop, got %q", vars["ARCH"].String())
	}
}

func TestExecFor_CtxCancelledMidLoop_ReturnsErrInterrupted(t *testing.T) {
	// given a loop: step with multiple slow iterations, when ctx is cancelled
	// partway through, then the loop stops early and the error wraps
	// ErrInterrupted (why: executeSteps' between-step guard must be reached
	// again on each loop iteration, not just checked once before the loop
	// starts)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"a", "b", "c", "d", "e"},
					ForSteps: []config.Step{
						{Kind: config.KindRun, Command: "sleep 0.1"},
					},
				},
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
