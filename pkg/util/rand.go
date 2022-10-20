package util

import (
	"crypto/rand"
	"math/big"
)

func GenerateRandomString(length int) []byte {
	chars := []byte("BCDFGHJKLMNPQRSTVWXZ" +
		"bcdfghjklmnpqrstvwxz" +
		"0123456789")
	b := make([]byte, length)
	for i := range b {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			panic(err)
		}
		b[i] = chars[index.Int64()]
	}
	return b
}
