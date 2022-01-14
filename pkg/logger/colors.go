package logger

import (
	"os"

	"github.com/jwalton/go-supportscolor"
	"github.com/ttacon/chalk"
)

var colorEnabled = supportscolor.SupportsColor(
	os.Stderr.Fd(),
	supportscolor.SniffFlagsOption(false),
).SupportsColor

// wrap chalk methods

func ColorEnabled() bool {
	return colorEnabled
}

func TextStyle(text string, textStyle chalk.TextStyle) string {
	if !colorEnabled {
		return text
	}
	return textStyle.TextStyle(text)
}

func Color(text string, color chalk.Color) string {
	if !colorEnabled {
		return text
	}
	return color.Color(text)
}
