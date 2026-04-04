package pcputil

import (
	"testing"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/version"
)

// TestBuildHostAtom_Basic は最小限のパラメータで正しい PCPHost アトムが構築されることを確認する。
func TestBuildHostAtom_Basic(t *testing.T) {
	sid := pcp.GnuID{1, 2, 3}
	cid := pcp.GnuID{4, 5, 6}
	atom := BuildHostAtom(HostAtomParams{
		SessionID:    sid,
		LocalIP:      0x0A000001, // 10.0.0.1
		GlobalIP:     0xC0A80001, // 192.168.0.1
		ListenPort:   7144,
		ChannelID:    cid,
		NumListeners: 3,
		NumRelays:    2,
		Uptime:       100,
		OldPos:       1000,
		NewPos:       2000,
		HasGlobalIP:  true,
	})

	if atom.Tag != pcp.PCPHost {
		t.Fatalf("tag: got %s, want %s", atom.Tag, pcp.PCPHost)
	}

	// Two ip/port pairs: local (1st) + global (2nd).
	ips := atom.FindChildren(pcp.PCPHostIP)
	if len(ips) != 2 {
		t.Fatalf("PCPHostIP: got %d atoms, want 2", len(ips))
	}
	if v, _ := ips[0].GetInt(); v != 0x0A000001 {
		t.Errorf("PCPHostIP (local): got 0x%08X, want 0x0A000001", v)
	}
	if v, _ := ips[1].GetInt(); v != 0xC0A80001 {
		t.Errorf("PCPHostIP (global): got 0x%08X, want 0xC0A80001", v)
	}
	ports := atom.FindChildren(pcp.PCPHostPort)
	if len(ports) != 2 {
		t.Fatalf("PCPHostPort: got %d atoms, want 2", len(ports))
	}
	for i, p := range ports {
		if v, _ := p.GetShort(); v != 7144 {
			t.Errorf("PCPHostPort[%d]: got %d, want 7144", i, v)
		}
	}

	// Check session ID.
	idAtom := atom.FindChild(pcp.PCPHostID)
	if idAtom == nil {
		t.Fatal("PCPHostID not found")
	}
	gotID, err := idAtom.GetID()
	if err != nil {
		t.Fatalf("GetID: %v", err)
	}
	if gotID != sid {
		t.Errorf("session ID: got %v, want %v", gotID, sid)
	}

	// Check channel ID.
	cidAtom := atom.FindChild(pcp.PCPHostChanID)
	if cidAtom == nil {
		t.Fatal("PCPHostChanID not found")
	}
	gotCID, err := cidAtom.GetID()
	if err != nil {
		t.Fatalf("GetID: %v", err)
	}
	if gotCID != cid {
		t.Errorf("channel ID: got %v, want %v", gotCID, cid)
	}

	// Check listeners count.
	nlAtom := atom.FindChild(pcp.PCPHostNumListeners)
	if nlAtom == nil {
		t.Fatal("PCPHostNumListeners not found")
	}
	if nl, err := nlAtom.GetInt(); err != nil || nl != 3 {
		t.Errorf("NumListeners: got %d, want 3", nl)
	}

	// Check relays count.
	nrAtom := atom.FindChild(pcp.PCPHostNumRelays)
	if nrAtom == nil {
		t.Fatal("PCPHostNumRelays not found")
	}
	if nr, err := nrAtom.GetInt(); err != nil || nr != 2 {
		t.Errorf("NumRelays: got %d, want 2", nr)
	}

	// Check version.
	verAtom := atom.FindChild(pcp.PCPHostVersion)
	if verAtom == nil {
		t.Fatal("PCPHostVersion not found")
	}
	if v, err := verAtom.GetInt(); err != nil || v != version.PCPVersion {
		t.Errorf("Version: got %d, want %d", v, version.PCPVersion)
	}
}

// TestBuildHostAtom_Flags はフラグが正しく設定されることを確認する。
func TestBuildHostAtom_Flags(t *testing.T) {
	tests := []struct {
		name        string
		hasGlobalIP bool
		isTracker   bool
		wantDirect  bool
		wantTracker bool
	}{
		{"no_global_no_tracker", false, false, false, false},
		{"global_no_tracker", true, false, true, false},
		{"no_global_tracker", false, true, false, true},
		{"global_and_tracker", true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atom := BuildHostAtom(HostAtomParams{
				HasGlobalIP: tt.hasGlobalIP,
				IsTracker:   tt.isTracker,
			})
			flagsAtom := atom.FindChild(pcp.PCPHostFlags1)
			if flagsAtom == nil {
				t.Fatal("PCPHostFlags1 not found")
			}
			flags, err := flagsAtom.GetByte()
			if err != nil {
				t.Fatalf("GetByte: %v", err)
			}

			hasDirect := flags&pcp.PCPHostFlags1Direct != 0
			if hasDirect != tt.wantDirect {
				t.Errorf("Direct flag: got %v, want %v", hasDirect, tt.wantDirect)
			}
			hasTracker := flags&pcp.PCPHostFlags1Tracker != 0
			if hasTracker != tt.wantTracker {
				t.Errorf("Tracker flag: got %v, want %v", hasTracker, tt.wantTracker)
			}
			// Relay, Recv, CIN should always be set.
			if flags&pcp.PCPHostFlags1Relay == 0 {
				t.Error("Relay flag should always be set")
			}
			if flags&pcp.PCPHostFlags1Recv == 0 {
				t.Error("Recv flag should always be set")
			}
		})
	}
}

// TestBuildHostAtom_TrackerAtom は TrackerAtom=true のとき PCPHostTracker が含まれることを確認する。
func TestBuildHostAtom_TrackerAtom(t *testing.T) {
	atom := BuildHostAtom(HostAtomParams{TrackerAtom: true})
	trackerAtom := atom.FindChild(pcp.PCPHostTracker)
	if trackerAtom == nil {
		t.Fatal("PCPHostTracker not found when TrackerAtom=true")
	}

	atom2 := BuildHostAtom(HostAtomParams{TrackerAtom: false})
	if atom2.FindChild(pcp.PCPHostTracker) != nil {
		t.Error("PCPHostTracker should not be present when TrackerAtom=false")
	}
}

// TestBuildHostAtom_UphostFields は UphostIP/Port 設定時に対応アトムが含まれることを確認する。
func TestBuildHostAtom_UphostFields(t *testing.T) {
	atom := BuildHostAtom(HostAtomParams{
		UphostIP:   0x0A000001,
		UphostPort: 7144,
		UphostHops: 2,
	})

	ipAtom := atom.FindChild(pcp.PCPHostUphostIP)
	if ipAtom == nil {
		t.Fatal("PCPHostUphostIP not found")
	}
	if ip, err := ipAtom.GetInt(); err != nil || ip != 0x0A000001 {
		t.Errorf("UphostIP: got %x, want 0x0A000001", ip)
	}

	hopsAtom := atom.FindChild(pcp.PCPHostUphostHops)
	if hopsAtom == nil {
		t.Fatal("PCPHostUphostHops not found")
	}
	if h, err := hopsAtom.GetInt(); err != nil || h != 2 {
		t.Errorf("UphostHops: got %d, want 2", h)
	}
}

// TestBuildHostAtom_NoUphost は Uphost 未設定時に対応アトムが含まれないことを確認する。
func TestBuildHostAtom_NoUphost(t *testing.T) {
	atom := BuildHostAtom(HostAtomParams{})

	if atom.FindChild(pcp.PCPHostUphostIP) != nil {
		t.Error("PCPHostUphostIP should not be present when uphost is zero")
	}
	if atom.FindChild(pcp.PCPHostUphostHops) != nil {
		t.Error("PCPHostUphostHops should not be present when uphost is zero")
	}
}
