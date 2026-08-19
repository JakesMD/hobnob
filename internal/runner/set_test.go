package runner

import (
	"context"
	"testing"

	"hobnob/internal/cli"
	"hobnob/internal/config"
)

func TestExecSet_SecretFlag(t *testing.T) {
	tests := []struct {
		name          string
		entries       []config.SetEntry
		wantSecrets   map[string]bool
		wantNoSecrets []string
	}{
		{
			name: "given secret:true set entry, when executed, then var marked in secrets (why: must be masked in run display)",
			entries: []config.SetEntry{
				{Key: "API_KEY", ValTmpl: "abc123", Secret: true},
			},
			wantSecrets: map[string]bool{"API_KEY": true},
		},
		{
			name: "given non-secret set entry, when executed, then var not in secrets (why: only explicitly marked vars masked)",
			entries: []config.SetEntry{
				{Key: "HOST", ValTmpl: "localhost"},
			},
			wantNoSecrets: []string{"HOST"},
		},
		{
			name: "given mixed set entries, when executed, then only secret ones marked (why: selective masking)",
			entries: []config.SetEntry{
				{Key: "TOKEN", ValTmpl: "s3cr3t", Secret: true},
				{Key: "ENV", ValTmpl: "prod"},
			},
			wantSecrets:   map[string]bool{"TOKEN": true},
			wantNoSecrets: []string{"ENV"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindSet, SetEntries: test.entries}}},
				},
			}
			secrets := make(map[string]bool)

			// Act
			err := ExecuteTask(context.Background(), "t", &cli.Scope{Vars: map[string]string{}, Secrets: secrets}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for key := range test.wantSecrets {
				if !secrets[key] {
					t.Errorf("secrets[%q]: want true, got false", key)
				}
			}
			for _, key := range test.wantNoSecrets {
				if secrets[key] {
					t.Errorf("secrets[%q]: want false, got true", key)
				}
			}
		})
	}
}
