package scope

import (
	"testing"

	"hobnob/internal/value"
)

func TestMask(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		vars    map[string]string
		secrets []string
		want    string
	}{
		{
			name:  "given no secrets, when masking, then string unchanged (why: nothing to mask)",
			input: "deploy --user=alice --pass=hunter2",
			vars:  map[string]string{"PASS": "hunter2"},
			want:  "deploy --user=alice --pass=hunter2",
		},
		{
			name:    "given secret var in command, when masking, then value replaced with **** (why: secret must not appear in logs)",
			input:   "deploy --user=alice --pass=hunter2",
			vars:    map[string]string{"PASS": "hunter2"},
			secrets: []string{"PASS"},
			want:    "deploy --user=alice --pass=****",
		},
		{
			name:    "given secret value appears multiple times, when masking, then all replaced (why: full redaction required)",
			input:   "echo hunter2 && login --pass=hunter2",
			vars:    map[string]string{"PASS": "hunter2"},
			secrets: []string{"PASS"},
			want:    "echo **** && login --pass=****",
		},
		{
			name:    "given secret var with empty value, when masking, then string unchanged (why: empty string replacement would corrupt output)",
			input:   "deploy --pass=",
			vars:    map[string]string{"PASS": ""},
			secrets: []string{"PASS"},
			want:    "deploy --pass=",
		},
		{
			name:    "given multiple secret vars, when masking, then all replaced (why: each secret must be redacted)",
			input:   "connect --user=root --pass=s3cr3t --token=abc123",
			vars:    map[string]string{"PASS": "s3cr3t", "TOKEN": "abc123"},
			secrets: []string{"PASS", "TOKEN"},
			want:    "connect --user=root --pass=**** --token=****",
		},
		{
			name:    `given secret containing a quote embedded in a JSON literal (json.Marshal-escaped), when masking, then the escaped form is also replaced (why: a set:/into: JSON literal leaf marshals its value, so the escaped form can differ from the raw secret and must be matched too)`,
			input:   `echo 'literal={"token":"ab\"cd"} raw=ab"cd'`,
			vars:    map[string]string{"TOK": `ab"cd`},
			secrets: []string{"TOK"},
			want:    `echo 'literal={"token":"****"} raw=****'`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			scope := New()
			for key, val := range test.vars {
				scope.Set(key, value.Str(val), false)
			}
			for _, key := range test.secrets {
				scope.MarkSecret(key)
			}

			// Act
			got := scope.Mask(test.input)

			// Assert
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// ambientScope returns a scope whose key holds val as if inherited from the
// OS env — the one state only Load's base layer produces.
func ambientScope(key, val string) *Scope {
	scope := New()
	scope.vars[key] = value.Str(val)
	scope.ambient[key] = true
	return scope
}

func TestSet_ClaimsAmbientName(t *testing.T) {
	// given a name inherited from the OS env, when Set overwrites it, then a
	// later SetIfDefault leaves it alone (why: any write above the OS env
	// claims the name, so a module's vars: default may not clobber a step's
	// write)

	// Arrange
	scope := ambientScope("EDITOR", "vim")

	// Act
	scope.Set("EDITOR", value.Str("from-set-step"), false)
	scope.SetIfDefault("EDITOR", value.Str("from-module-vars"), false)

	// Assert
	if got := scope.Vars()["EDITOR"].String(); got != "from-set-step" {
		t.Errorf("EDITOR = %q, want %q", got, "from-set-step")
	}
}

func TestBind_RestoresPriorState(t *testing.T) {
	tests := []struct {
		name      string
		scope     func() *Scope
		wantValue string
		wantHad   bool
		wantFill  bool
	}{
		{
			name:     "given ITEM absent, when a Bind is restored, then ITEM is absent again (why: a loop leaves no iterator var behind)",
			scope:    New,
			wantHad:  false,
			wantFill: true,
		},
		{
			name:      "given ITEM inherited from the OS env, when a Bind is restored, then ITEM is ambient again (why: a loop must not claim a name it only borrowed, or a module's vars: default could no longer fill it)",
			scope:     func() *Scope { return ambientScope("ITEM", "from-env") },
			wantValue: "module-default",
			wantHad:   true,
			wantFill:  true,
		},
		{
			name: "given ITEM set by a step, when a Bind is restored, then ITEM keeps its value and stays claimed (why: the restore puts back exactly what was there)",
			scope: func() *Scope {
				scope := New()
				scope.Set("ITEM", value.Str("from-step"), false)
				return scope
			},
			wantValue: "from-step",
			wantHad:   true,
			wantFill:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			scope := test.scope()

			// Act
			restore := scope.Bind("ITEM", value.Str("iteration"))
			restore()
			_, had := scope.Vars()["ITEM"]
			scope.SetIfDefault("ITEM", value.Str("module-default"), false)

			// Assert
			if test.wantHad != had {
				t.Errorf("ITEM present after restore = %v, want %v", had, test.wantHad)
			}
			got := scope.Vars()["ITEM"].String()
			filled := got == "module-default"
			if filled != test.wantFill {
				t.Errorf("SetIfDefault filled ITEM = %v (ITEM = %q), want %v", filled, got, test.wantFill)
			}
			if test.wantValue != "" && got != test.wantValue {
				t.Errorf("ITEM = %q, want %q", got, test.wantValue)
			}
		})
	}
}

func TestBind_DoesNotLeakSecretFlag(t *testing.T) {
	// given ITEM marked secret during one iteration, when that Bind is
	// restored, then ITEM's value no longer gets masked (why: a restore puts
	// back the secret flag it found, not the one an iteration left)

	// Arrange
	scope := New()

	// Act
	restore := scope.Bind("ITEM", value.Str("hunter2"))
	scope.MarkSecret("ITEM")
	restore()
	scope.Set("ITEM", value.Str("public"), false)

	// Assert
	if got := scope.Mask("echo public"); got != "echo public" {
		t.Errorf("Mask = %q, want unmasked", got)
	}
}
