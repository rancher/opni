package test

import (
	"github.com/goombaio/namegenerator"
)

func RandomName(seed int64) string {
	nameGenerator := namegenerator.NewNameGenerator(seed)
	return nameGenerator.Generate()
}
