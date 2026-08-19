package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"hobnob/internal/config"
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
		initVars   map[string]string
		wantResult string
	}{
		{
			name: "given single-var map form, when executed, then iterates using named variable (why: map form names iterator without as:)",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos", "windows"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " ubuntu macos windows",
		},
		{
			name: "given two-var matrix, when executed, then runs full cartesian product",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}/{{.DB}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " ubuntu/postgres ubuntu/sqlite macos/postgres macos/sqlite",
		},
		{
			name: "given two-var matrix with reversed key order, when executed, then second key becomes inner loop (why: first key is outermost per spec)",
			matrix: []config.ForMatrixEntry{
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
			},
			innerTmpl:  "{{.RESULT}} {{.DB}}/{{.PLATFORM}}",
			initVars:   map[string]string{"RESULT": ""},
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
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " linux/amd64/pg linux/arm64/pg macos/amd64/pg macos/arm64/pg",
		},
		{
			name: "given single-var matrix with one item, when executed, then runs exactly once",
			matrix: []config.ForMatrixEntry{
				{VarName: "ENV", List: []string{"prod"}},
			},
			innerTmpl:  "{{.RESULT}}{{.ENV}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: "prod",
		},
		{
			name: "given matrix with dynamic list template, when executed, then resolves list from vars",
			matrix: []config.ForMatrixEntry{
				{VarName: "NODE", ListTmpl: `{{.SERVERS}}`},
			},
			innerTmpl:  "{{.RESULT}} {{.NODE}}",
			initVars:   map[string]string{"RESULT": "", "SERVERS": `["web1","web2"]`},
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
			if vars["RESULT"] != test.wantResult {
				t.Errorf("RESULT: got %q, want %q", vars["RESULT"], test.wantResult)
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
		initVars   map[string]string
		wantResult string
	}{
		{
			name:       "given literal list, when executed, then iterates binding each item to ITEM (why: string form uses ITEM as default iterator)",
			forList:    []string{"alpha", "beta", "gamma"},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " alpha beta gamma",
		},
		{
			name:       "given template target resolving to JSON array, when executed, then iterates resolved items (why: dynamic lists evaluated at runtime)",
			forTarget:  "{{.FILES}}",
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": "", "FILES": `["x","y","z"]`},
			wantResult: " x y z",
		},
		{
			name:       "given empty list, when executed, then runs zero iterations (why: empty source must not error)",
			forList:    []string{},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": "initial"},
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
			if vars["RESULT"] != test.wantResult {
				t.Errorf("RESULT: got %q, want %q", vars["RESULT"], test.wantResult)
			}
		})
	}
}

func TestExecFor_Map(t *testing.T) {
	// given loop: target resolves to a JSON object, when executed, then iterates sorted keys binding KEY/VALUE (why: map iteration is the object counterpart to list iteration's ITEM)
	// Arrange
	cfg := makeForStringCfg(nil, "{{.REGIONS}}", "{{.RESULT}} {{.KEY}}={{.VALUE}}")
	vars := copyVars(map[string]string{"RESULT": "", "REGIONS": `{"us":"us-east-1","eu":"eu-west-1"}`})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != " eu=eu-west-1 us=us-east-1" {
		t.Errorf("RESULT: got %q, want %q", vars["RESULT"], " eu=eu-west-1 us=us-east-1")
	}
}

func TestExecFor_Map_MalformedJSON(t *testing.T) {
	// given loop: target resolves to a string starting with { that isn't valid JSON, when executed, then returns error (why: fail fast rather than silently skipping the loop)
	// Arrange
	cfg := makeForStringCfg(nil, "{{.BAD}}", "{{.RESULT}}{{.KEY}}")
	vars := copyVars(map[string]string{"RESULT": "", "BAD": `{not valid json`})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
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
	vars := map[string]string{"RESULT": "", "MAP": `{"a":"1"}`}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := vars["KEY"]; exists {
		t.Errorf("KEY should not exist after loop, got %q", vars["KEY"])
	}
	if _, exists := vars["VALUE"]; exists {
		t.Errorf("VALUE should not exist after loop, got %q", vars["VALUE"])
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
	vars := map[string]string{"RESULT": ""}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != "abc" {
		t.Errorf("RESULT: got %q, want abc", vars["RESULT"])
	}
	if _, exists := vars["ITEM"]; exists {
		t.Errorf("ITEM should not exist after loop, got %q", vars["ITEM"])
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
	vars := map[string]string{"ITEM": "original", "RESULT": ""}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["ITEM"] != "original" {
		t.Errorf("ITEM: got %q, want original (should be restored)", vars["ITEM"])
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
	vars := map[string]string{"RESULT": ""}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := vars["OS"]; exists {
		t.Errorf("OS should not exist after matrix loop, got %q", vars["OS"])
	}
	if _, exists := vars["ARCH"]; exists {
		t.Errorf("ARCH should not exist after matrix loop, got %q", vars["ARCH"])
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
	err := ExecuteTask(ctx, "t", makeScope(map[string]string{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
