package site

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"
)

func TestShortKeyIssuanceAndRotation(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	b := addSession(s, "bob", "2")
	for _, ss := range []*session{a, b} {
		cookie := "alice"
		if ss == b {
			cookie = "bob"
		}
		w := call(s, "POST", "/site/api/key", cookie, ss.CSRF, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result struct {
			Key string `json:"streamKey"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^[a-z]{2}[0-9]{4}$`).MatchString(result.Key) {
			t.Fatal("incorrect short key format")
		}
	}
	old := s.key(a)
	if old == s.key(b) {
		t.Fatal("duplicate keys")
	}
	w := call(s, "POST", "/site/api/key", "alice", a.CSRF, "")
	if w.Code != 200 || s.key(a) == old || s.mgr.IsIssuedKey(old) {
		t.Fatal("rotation did not replace key")
	}
}

func TestShortKeyCollisionAndFailure(t *testing.T) {
	s := testSite(t)
	if err := s.mgr.IssueStreamKey("alice", "aa1111"); err != nil {
		t.Fatal(err)
	}
	if err := s.mgr.IssueStreamKey("bob", "bb2222"); err != nil {
		t.Fatal(err)
	}
	candidates := []string{"aa1111", "bb2222", "cc3333"}
	index := 0
	key, err := s.issueShortKey("alice", "aa1111", func() (string, error) { v := candidates[index]; index++; return v, nil })
	if err != nil || key != "cc3333" || index != 3 {
		t.Fatal(key, err, index)
	}
	if !s.mgr.IsIssuedKey("bb2222") || s.mgr.IsIssuedKey("aa1111") {
		t.Fatal("ownership or old key changed incorrectly")
	}
	if _, err = s.issueShortKey("alice", key, func() (string, error) { return "", errors.New("entropy unavailable") }); err == nil {
		t.Fatal("ignored RNG failure")
	}
	if !s.mgr.IsIssuedKey(key) {
		t.Fatal("failed generation revoked key")
	}
	if _, err = s.issueShortKey("alice", key, func() (string, error) { return "bb2222", nil }); err == nil {
		t.Fatal("unbounded collision retry")
	}
}

func TestExistingLongKeySurvivesUntilRotation(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	const old = "previous-long-key"
	if err := s.mgr.IssueStreamKey(account(a), old); err != nil {
		t.Fatal(err)
	}
	call(s, "GET", "/site/api/broadcast", "alice", "", "")
	if s.key(a) != old {
		t.Fatal("read rotated old key")
	}
}
