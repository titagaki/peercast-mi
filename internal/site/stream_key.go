package site

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Uniformly sample all 26*26*10000 keys without modulo bias.
func shortStreamKey() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(26*26*10000))
	if err != nil {
		return "", err
	}
	n := value.Int64()
	return fmt.Sprintf("%c%c%04d", 'a'+n/(26*10000), 'a'+(n/10000)%26, n%10000), nil
}
