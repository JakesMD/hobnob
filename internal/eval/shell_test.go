package eval

import (
	"context"
	"os"
	"testing"

	"hobnob/internal/value"
)

func TestEvalCondition(t *testing.T) {
	tests := []struct {
		name      string
		condTmpl  string
		vars      map[string]value.Value
		wantTrue  bool
		wantError bool
	}{
		{
			name:     "given string equality match, when evaluated, then returns true (why: if condition gates step execution)",
			condTmpl: `[ "{{.METHOD}}" = "Chunked Upload" ]`,
			vars:     map[string]value.Value{"METHOD": value.Str("Chunked Upload")},
			wantTrue: true,
		},
		{
			name:     "given string equality mismatch, when evaluated, then returns false (why: step should be skipped)",
			condTmpl: `[ "{{.METHOD}}" = "Chunked Upload" ]`,
			vars:     map[string]value.Value{"METHOD": value.Str("Direct Upload")},
			wantTrue: false,
		},
		{
			name:     "given string inequality match, when evaluated, then returns true (why: != skips excluded value)",
			condTmpl: `[ "{{.MOTOR}}" != "Z-Axis Lead" ]`,
			vars:     map[string]value.Value{"MOTOR": value.Str("X-Axis Stepper")},
			wantTrue: true,
		},
		{
			name:     "given string inequality match on excluded value, when evaluated, then returns false (why: Z-Axis Lead is excluded)",
			condTmpl: `[ "{{.MOTOR}}" != "Z-Axis Lead" ]`,
			vars:     map[string]value.Value{"MOTOR": value.Str("Z-Axis Lead")},
			wantTrue: false,
		},
		{
			name:     "given numeric less-than-equal pass, when evaluated, then returns true (why: check validates numeric bounds)",
			condTmpl: `[ {{.SPEED}} -le {{.LIMIT}} ]`,
			vars:     map[string]value.Value{"SPEED": value.Str("1500"), "LIMIT": value.Str("3000")},
			wantTrue: true,
		},
		{
			name:     "given numeric less-than-equal fail, when evaluated, then returns false (why: value exceeds limit)",
			condTmpl: `[ {{.SPEED}} -le {{.LIMIT}} ]`,
			vars:     map[string]value.Value{"SPEED": value.Str("9999"), "LIMIT": value.Str("3000")},
			wantTrue: false,
		},
		{
			name:     "given numeric less-than pass, when evaluated, then returns true (why: chunk size within limit)",
			condTmpl: `[ {{.SIZE}} -lt {{.MAX}} ]`,
			vars:     map[string]value.Value{"SIZE": value.Str("1024"), "MAX": value.Str("10485760")},
			wantTrue: true,
		},
		{
			name:     "given numeric less-than fail, when evaluated, then returns false (why: oversized chunk must be rejected)",
			condTmpl: `[ {{.SIZE}} -lt {{.MAX}} ]`,
			vars:     map[string]value.Value{"SIZE": value.Str("99999999"), "MAX": value.Str("10485760")},
			wantTrue: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalCondition(context.Background(), test.condTmpl, test.vars, "")

			// Assert
			if test.wantError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.wantTrue {
				t.Errorf("got %v, want %v", got, test.wantTrue)
			}
		})
	}
}

func TestEvalCheckWithOverride(t *testing.T) {
	t.Run("given a candidate value passing the check, when evaluated, then returns true (why: re-prompt loops commit a value only once its check passes)", func(t *testing.T) {
		// Arrange
		vars := map[string]value.Value{"PORT": value.Str("80")}

		// Act
		ok, err := EvalCheckWithOverride(context.Background(), `[ {{.PORT}} -gt 1024 ]`, vars, "PORT", value.Str("8080"))

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("got false, want true — check should see the override, not the scope value")
		}
	})

	t.Run("given a candidate value failing the check, when evaluated, then returns false (why: a bad selection must re-prompt rather than commit)", func(t *testing.T) {
		// Arrange
		vars := map[string]value.Value{"PORT": value.Str("8080")}

		// Act
		ok, err := EvalCheckWithOverride(context.Background(), `[ {{.PORT}} -gt 1024 ]`, vars, "PORT", value.Str("80"))

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("got true, want false — check should see the override, not the scope value")
		}
	})

	t.Run("given a check evaluated against a candidate, when it returns, then the caller's vars are unchanged (why: prompts probe candidate values before committing — a mutated scope would leak a rejected value)", func(t *testing.T) {
		// Arrange
		vars := map[string]value.Value{"PORT": value.Str("80"), "HOST": value.Str("localhost")}

		// Act
		if _, err := EvalCheckWithOverride(context.Background(), `[ {{.PORT}} -gt 1024 ]`, vars, "PORT", value.Str("8080")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Assert
		if vars["PORT"].String() != "80" {
			t.Errorf("PORT mutated: got %q, want 80", vars["PORT"].String())
		}
		if len(vars) != 2 {
			t.Errorf("vars gained or lost keys: %v", vars)
		}
	})

	t.Run("given a key absent from vars, when evaluated, then the override supplies it without adding it to the caller's map (why: an optional prompt's var may not exist in scope yet)", func(t *testing.T) {
		// Arrange
		vars := map[string]value.Value{}

		// Act
		ok, err := EvalCheckWithOverride(context.Background(), `[ "{{.CHOICE}}" = "good" ]`, vars, "CHOICE", value.Str("good"))

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("got false, want true")
		}
		if _, exists := vars["CHOICE"]; exists {
			t.Error("CHOICE leaked into the caller's vars map")
		}
	})
}

func TestSourceShellFile(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		presetEnv  map[string]string
		wantVar    string
		wantValue  string
		wantAbsent bool
	}{
		{
			name:      "given script exports a new var, when sourced, then var is captured (why: the whole point of env: is picking up vars a script defines)",
			script:    "export NEWVAR=hello\n",
			wantVar:   "NEWVAR",
			wantValue: "hello",
		},
		{
			name:       "given script re-exports an existing var unchanged, when sourced, then var is not reported as set (why: diffing against baseline filters out ambient noise)",
			script:     "export SAMEVAR=same\n",
			presetEnv:  map[string]string{"SAMEVAR": "same"},
			wantVar:    "SAMEVAR",
			wantAbsent: true,
		},
		{
			name:      "given script changes an existing var's value, when sourced, then the new value is captured (why: a script overriding an inherited var is a deliberate change)",
			script:    "export CHANGEDVAR=newvalue\n",
			presetEnv: map[string]string{"CHANGEDVAR": "oldvalue"},
			wantVar:   "CHANGEDVAR",
			wantValue: "newvalue",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			for key, value := range test.presetEnv {
				t.Setenv(key, value)
			}
			dir := t.TempDir()
			scriptPath := dir + "/script.sh"
			if err := os.WriteFile(scriptPath, []byte(test.script), 0o644); err != nil {
				t.Fatalf("write script: %v", err)
			}

			// Act
			got, err := SourceShellFile(context.Background(), scriptPath, dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Assert
			if test.wantAbsent {
				if _, ok := got[test.wantVar]; ok {
					t.Errorf("%s: expected absent from diff, got %q", test.wantVar, got[test.wantVar])
				}
				return
			}
			if got[test.wantVar] != test.wantValue {
				t.Errorf("%s: got %q, want %q", test.wantVar, got[test.wantVar], test.wantValue)
			}
		})
	}
}

func TestSourceShellFile_PathWithShellMetacharactersNotExecuted(t *testing.T) {
	// given a path containing shell command substitution syntax, when sourced, then the substitution is not executed (why: a templated env: path built from an untrusted var must not let shell metacharacters run arbitrary commands)
	// Arrange
	dir := t.TempDir()
	scriptName := "$(touch injected).sh"
	scriptPath := dir + "/" + scriptName
	if err := os.WriteFile(scriptPath, []byte("export FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	// Act
	got, err := SourceShellFile(context.Background(), scriptPath, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
	if _, err := os.Stat(dir + "/injected"); err == nil {
		t.Error("canary file exists: command substitution in path was executed")
	}
}

func TestSourceShellFile_MultiLineValuePreserved(t *testing.T) {
	// given a script exporting a value with embedded newlines, when sourced, then the full multi-line value is captured (why: env: files are used to load secrets like PEM certs/keys, which are commonly multi-line)
	// Arrange
	dir := t.TempDir()
	script := "export CERT=\"$(printf 'line1\\nline2\\nline3')\"\n"
	scriptPath := dir + "/script.sh"
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	// Act
	got, err := SourceShellFile(context.Background(), scriptPath, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	want := "line1\nline2\nline3"
	if got["CERT"] != want {
		t.Errorf("CERT: got %q, want %q", got["CERT"], want)
	}
}
