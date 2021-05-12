package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	if err := os.RemoveAll("./pkg/generated"); err != nil {
		logrus.Fatal(err)
	}
}
