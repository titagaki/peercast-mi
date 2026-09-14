package id

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/titagaki/peercast-pcp/pcp"
)

// LoadOrCreateBroadcastID persists a stable seed. Never replace corrupt state
// silently: doing so would change every broadcast URL after a restart.
func LoadOrCreateBroadcastID(path string) (pcp.GnuID, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		b, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(b) != 16 {
			return pcp.GnuID{}, fmt.Errorf("invalid broadcast ID in %s", path)
		}
		var id pcp.GnuID
		copy(id[:], b)
		if id.IsEmpty() {
			return id, fmt.Errorf("empty broadcast ID in %s", path)
		}
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return pcp.GnuID{}, err
	}
	id := NewRandom()
	id[0] &= 0xFE // PeerCast broadcast identifiers reserve the low bit.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".broadcast-id-*")
	if err != nil {
		return pcp.GnuID{}, err
	}
	defer os.Remove(tmp.Name())
	_, err = fmt.Fprintf(tmp, "%x\n", id[:])
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return pcp.GnuID{}, err
	}
	if closeErr != nil {
		return pcp.GnuID{}, closeErr
	}
	// Link publishes the complete file without overwriting a concurrent creator.
	if err := os.Link(tmp.Name(), path); errors.Is(err, os.ErrExist) {
		return LoadOrCreateBroadcastID(path)
	} else if err != nil {
		return pcp.GnuID{}, err
	}
	return id, nil
}
