package dateparser

import (
	"regexp"
	"strings"
	"time"
)

const (
	datetimeRegexISO8601 = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{9}Z`
)

// Default parser will parse logs with a default ISO8601 timestamp.
type DefaultParser struct {
	TimestampRegex string
}

func (p *DefaultParser) ParseTimestamp(log string) (time.Time, string, bool) {
	re := regexp.MustCompile(datetimeRegexISO8601)
	datestring := re.FindString(log)
	datetime, err := time.Parse(time.RFC3339Nano, datestring)
	if err != nil {
		panic(err)
	}

	cleaned := strings.TrimSpace(re.ReplaceAllString(log, ""))

	re = regexp.MustCompile(p.TimestampRegex)
	datestring = re.FindString(cleaned)
	valid := len(datestring) > 0

	return datetime, cleaned, valid
}
