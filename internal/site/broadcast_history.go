package site

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Only successful form submissions are templates; runtime metadata and keys
// are deliberately excluded. All access is serialized by Server.mutate.
type broadcastSettings struct {
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Comment     string `json:"comment"`
	ContactURL  string `json:"contactUrl"`
}
type broadcastHistoryEntry struct {
	broadcastSettings
	CreatedAt time.Time `json:"createdAt"`
}
type broadcastHistoryFile struct {
	Version int                     `json:"version"`
	Entries []broadcastHistoryEntry `json:"entries"`
}

func (s *Server) historyPath(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return filepath.Join(s.cfg.BroadcastHistoryDir, hex.EncodeToString(sum[:])+".json")
}
func (s *Server) readHistory(owner string) ([]broadcastHistoryEntry, error) {
	f, err := os.Open(s.historyPath(owner))
	if errors.Is(err, os.ErrNotExist) {
		return []broadcastHistoryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data broadcastHistoryFile
	d := json.NewDecoder(io.LimitReader(f, 512<<10))
	if err := d.Decode(&data); err != nil {
		return nil, err
	}
	if data.Version != 1 || len(data.Entries) > 10 {
		return nil, errors.New("invalid broadcast history")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("invalid broadcast history trailing data")
	}
	if data.Entries == nil {
		data.Entries = []broadcastHistoryEntry{}
	}
	return data.Entries, nil
}
func (s *Server) saveHistory(owner string, settings broadcastSettings, previous []broadcastHistoryEntry) error {
	entries := append([]broadcastHistoryEntry{{settings, time.Now().UTC()}}, previous...)
	if len(entries) > 10 {
		entries = entries[:10]
	}
	data, err := json.Marshal(broadcastHistoryFile{1, entries})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.cfg.BroadcastHistoryDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.cfg.BroadcastHistoryDir, ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), s.historyPath(owner))
}
