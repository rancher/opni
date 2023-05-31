package dateparser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// KlogDateZoneParser will parse a timestamp string without a year and timezone into a datetime
// given the regex and layout of the timestamp.  If the regex is not the default KlogRegex,
// it will check the log line also contains a timestamp matching the KlogRegex.
type KlogDateZoneParser struct {
	dateZoneOptions
	datetimeRegex string
	layout        string
}

type dateZoneOptions struct {
	timezone string
	year     string
}

type dateZoneOption func(*dateZoneOptions)

func (d *dateZoneOptions) apply(opts ...dateZoneOption) {
	for _, opt := range opts {
		opt(d)
	}
}

func WithTimezone(timezone string) dateZoneOption {
	return func(d *dateZoneOptions) {
		d.timezone = timezone
	}
}

func WithYear(year string) dateZoneOption {
	return func(d *dateZoneOptions) {
		d.year = year
	}
}

func NewDateZoneParser(datetimeRegex string, layout string, opts ...dateZoneOption) DateParser {
	options := dateZoneOptions{
		timezone: "UTC",
		year:     fmt.Sprint((time.Now().Year())),
	}
	options.apply(opts...)
	return &KlogDateZoneParser{
		dateZoneOptions: options,
		datetimeRegex:   datetimeRegex,
		layout:          layout,
	}
}

func (d *KlogDateZoneParser) ParseTimestamp(log string) (time.Time, string, bool) {
	re := regexp.MustCompile(d.datetimeRegex)
	datestring := re.FindString(log)
	if len(datestring) == 0 {
		return time.Now(), log, false
	}
	datetime, err := time.Parse(d.layout, fmt.Sprintf("%s %s %s", datestring, d.timezone, d.year))
	if err != nil {
		panic(err)
	}

	retLog := log
	valid := true

	if d.datetimeRegex != KlogRegex {
		cleaned := strings.TrimSpace(re.ReplaceAllString(log, ""))
		retLog = cleaned

		re = regexp.MustCompile(KlogRegex)
		datestring = re.FindString(cleaned)
		valid = len(datestring) > 0
	}

	return datetime, retLog, valid
}
