package util

import (
	"math/rand"
	"time"
)

func GenerateRandomString(length int) []byte {
	rand.Seed(time.Now().UnixNano())
	chars := []byte("BCDFGHJKLMNPQRSTVWXZ" +
		"bcdfghjklmnpqrstvwxz" +
		"0123456789")
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return b
}
