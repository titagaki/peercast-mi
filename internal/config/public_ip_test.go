package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublicIPv4Config(t *testing.T) {
	for _, tc := range []struct {
		ip    string
		valid bool
	}{
		{"", true}, {"153.127.50.98", true}, {"172.20.0.4", false}, {"127.0.0.1", false}, {"0.0.0.0", false}, {"224.0.0.1", false}, {"::1", false}, {"yayaue.me", false},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("public_ipv4 = \""+tc.ip+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
