package util

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

func UUIDToBigInt(uuid uuid.UUID) (*big.Int, error) {
	return UUIDStrToBigInt(uuid.String())
}

func UUIDStrToBigInt(uuid string) (*big.Int, error) {
	uuidStr := strings.ReplaceAll(uuid, "-", "")
	bigInt := new(big.Int)
	_, success := bigInt.SetString(uuidStr, 16)
	if !success {
		return nil, fmt.Errorf("failed to convert UUID to big int")
	}

	return bigInt, nil
}

func XorBigInt(a, b *big.Int) *big.Int {
	// Get the bits representation of the two big integers
	aBits := a.Bits()
	bBits := b.Bits()

	// Determine the maximum length for the slice (number of words)
	maxLen := len(aBits)
	if len(bBits) > maxLen {
		maxLen = len(bBits)
	}

	// Create a new slice to store the XOR result
	resultBits := make([]big.Word, maxLen)

	// Perform XOR operation on corresponding bits
	for i := 0; i < maxLen; i++ {
		var wordA, wordB big.Word
		if i < len(aBits) {
			wordA = aBits[i]
		}
		if i < len(bBits) {
			wordB = bBits[i]
		}
		resultBits[i] = wordA ^ wordB
	}

	// Create a new big.Int from the XOR result
	result := new(big.Int).SetBits(resultBits)

	return result
}
