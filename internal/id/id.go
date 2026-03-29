package id

import (
	"crypto/rand"

	"github.com/titagaki/peercast-pcp/pcp"
)

// NewRandom generates a random 16-byte GnuID.
func NewRandom() pcp.GnuID {
	var id pcp.GnuID
	if _, err := rand.Read(id[:]); err != nil {
		panic("id: failed to read random bytes: " + err.Error())
	}
	return id
}

// ChannelID generates a channel ID compatible with peercast-yt.
//
//	id の初期値 = broadcastID
//	1. 全バイトに bitrate(uint8) を XOR
//	2. name と genre の文字を循環しながら XOR
//	   ループ回数 = (max(len(name), len(genre))/16 + 1) * 16
//	   各文字列は末尾に達したらインデックスを 0 にリセットし、その回はXORしない
func ChannelID(broadcastID pcp.GnuID, name, genre string, bitrate uint32) pcp.GnuID {
	id := broadcastID

	// step 1: XOR with bitrate (lower 8 bits)
	b := byte(bitrate)
	for i := range id {
		id[i] ^= b
	}

	// step 2: XOR with name and genre cycling
	maxLen := len(name)
	if len(genre) > maxLen {
		maxLen = len(genre)
	}
	n := (maxLen/16 + 1) * 16
	s1, s2 := 0, 0
	for i := 0; i < n; i++ {
		ipb := id[i%16]
		if s1 < len(name) {
			ipb ^= name[s1]
			s1++
		} else {
			s1 = 0
		}
		if s2 < len(genre) {
			ipb ^= genre[s2]
			s2++
		} else {
			s2 = 0
		}
		id[i%16] = ipb
	}
	return id
}
