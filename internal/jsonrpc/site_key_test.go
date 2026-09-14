package jsonrpc

import (
	"encoding/json"
	"testing"
)

func TestIssueStreamKeyRejectsDuplicateOwner(t *testing.T) {
	s, _, _ := newTestServer(t)
	for _, args := range []string{`["site:x:1","site-key"]`, `["site:x:2","other-key"]`} {
		if _, e := s.issueStreamKey(json.RawMessage(args)); e != nil {
			t.Fatal(e)
		}
	}
	_, e := s.issueStreamKey(json.RawMessage(`["site:x:2","site-key"]`))
	if e == nil || e.Code != errCodeInternal || e.Message != "stream key already assigned" {
		t.Fatalf("unexpected error: %+v", e)
	}
}
