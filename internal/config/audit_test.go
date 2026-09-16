package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{"[audit]\nenabled=true\nnode_id='mi-production'", true},
		{"[audit]\nenabled=true", false},
		{"[audit]\nenabled=true\nnode_id='日本語'", false},
		{"[audit]\nenabled=true\nnode_id='mi'\nretention_days=-1", false},
		{"[audit]\nenabled=false", true},
	} {
		p := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(p, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
	}
}
