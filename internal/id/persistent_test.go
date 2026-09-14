package id

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/titagaki/peercast-pcp/pcp"
)

func TestPersistentBroadcastID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broadcast_id")
	var wg sync.WaitGroup
	ids := make(chan pcp.GnuID, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := LoadOrCreateBroadcastID(path)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	want, err := LoadOrCreateBroadcastID(path)
	if err != nil {
		t.Fatal(err)
	}
	if want.IsEmpty() || want[0]&1 != 0 {
		t.Fatal("invalid generated ID")
	}
	for id := range ids {
		if id != want {
			t.Fatal("concurrent creators disagree")
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatal("ID must be private")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateBroadcastID(path); err == nil {
		t.Fatal("corrupt state accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken" {
		t.Fatal("corrupt state overwritten")
	}
}
