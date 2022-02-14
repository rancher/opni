package logger

import (
	"os"

	"github.com/jwalton/go-supportscolor"
	"github.com/ttacon/chalk"
)

var colorEnabled = supportscolor.SupportsColor(
	os.Stdout.Fd(),
	supportscolor.SniffFlagsOption(false),
).SupportsColor

func ColorEnabled() bool {
	return colorEnabled || inTest
}

func TextStyle(text string, textStyle chalk.TextStyle) string {
	if !ColorEnabled() {
		return text
	}
	return textStyle.TextStyle(text)
}

func Color(text string, color chalk.Color) string {
	if !ColorEnabled() {
		return text
	}
	return color.Color(text)
}
