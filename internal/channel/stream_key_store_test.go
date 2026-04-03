package channel

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) (*StreamKeyStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stream_keys.json")
	s := NewStreamKeyStore()
	s.SetCachePath(path)
	return s, path
}

// TestStreamKeyStore_IssueAndLookup は Issue → IsIssuedKey の基本フローを確認する。
func TestStreamKeyStore_IssueAndLookup(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.IssueStreamKey("alice", "sk_1"); err != nil {
		t.Fatalf("IssueStreamKey: %v", err)
	}
	if !s.IsIssuedKey("sk_1") {
		t.Error("IsIssuedKey(sk_1): got false, want true")
	}
	if s.IsIssuedKey("sk_unknown") {
		t.Error("IsIssuedKey(sk_unknown): got true, want false")
	}
}

// TestStreamKeyStore_Overwrite は同じアカウントで再発行すると旧キーが無効化されることを確認する。
func TestStreamKeyStore_Overwrite(t *testing.T) {
	s, _ := newTestStore(t)

	s.IssueStreamKey("alice", "sk_old")
	s.IssueStreamKey("alice", "sk_new")

	if s.IsIssuedKey("sk_old") {
		t.Error("IsIssuedKey(sk_old): got true after overwrite, want false")
	}
	if !s.IsIssuedKey("sk_new") {
		t.Error("IsIssuedKey(sk_new): got false, want true")
	}
}

// TestStreamKeyStore_Revoke は Revoke 後にキーが無効になることを確認する。
func TestStreamKeyStore_Revoke(t *testing.T) {
	s, _ := newTestStore(t)

	s.IssueStreamKey("alice", "sk_1")

	if !s.RevokeStreamKey("alice") {
		t.Error("RevokeStreamKey(alice): got false, want true")
	}
	if s.IsIssuedKey("sk_1") {
		t.Error("IsIssuedKey after revoke: got true, want false")
	}
}

// TestStreamKeyStore_Revoke_NotFound は未登録アカウントで false を返すことを確認する。
func TestStreamKeyStore_Revoke_NotFound(t *testing.T) {
	s, _ := newTestStore(t)

	if s.RevokeStreamKey("nobody") {
		t.Error("RevokeStreamKey(nobody): got true, want false")
	}
}

// TestStreamKeyStore_CachePersistence は Issue したキーがキャッシュファイルに永続化され、
// 新しい StreamKeyStore で LoadCache すると復元されることを確認する。
func TestStreamKeyStore_CachePersistence(t *testing.T) {
	s, path := newTestStore(t)

	s.IssueStreamKey("alice", "sk_a")
	s.IssueStreamKey("bob", "sk_b")

	// Verify the cache file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Load into a fresh store.
	s2 := NewStreamKeyStore()
	s2.SetCachePath(path)
	if err := s2.LoadCache(); err != nil {
		t.Fatalf("LoadCache: %v", err)
	}

	if !s2.IsIssuedKey("sk_a") {
		t.Error("IsIssuedKey(sk_a) after reload: got false, want true")
	}
	if !s2.IsIssuedKey("sk_b") {
		t.Error("IsIssuedKey(sk_b) after reload: got false, want true")
	}
}

// TestStreamKeyStore_CacheAfterRevoke は Revoke 後にキャッシュから削除されることを確認する。
func TestStreamKeyStore_CacheAfterRevoke(t *testing.T) {
	s, path := newTestStore(t)

	s.IssueStreamKey("alice", "sk_a")
	s.RevokeStreamKey("alice")

	s2 := NewStreamKeyStore()
	s2.SetCachePath(path)
	if err := s2.LoadCache(); err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if s2.IsIssuedKey("sk_a") {
		t.Error("IsIssuedKey(sk_a) after revoke+reload: got true, want false")
	}
}

// TestStreamKeyStore_LoadCache_NoFile はキャッシュファイルが存在しない場合にエラーにならないことを確認する。
func TestStreamKeyStore_LoadCache_NoFile(t *testing.T) {
	s := NewStreamKeyStore()
	s.SetCachePath(filepath.Join(t.TempDir(), "nonexistent.json"))

	if err := s.LoadCache(); err != nil {
		t.Errorf("LoadCache(no file): unexpected error: %v", err)
	}
}

// TestStreamKeyStore_LoadCache_NoCachePath は cachePath 未設定で何もしないことを確認する。
func TestStreamKeyStore_LoadCache_NoCachePath(t *testing.T) {
	s := NewStreamKeyStore()
	if err := s.LoadCache(); err != nil {
		t.Errorf("LoadCache(no path): unexpected error: %v", err)
	}
}
