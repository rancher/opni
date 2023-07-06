package dateparser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DayMonthParser will parse a timestamp string without a year and timezone into a datetime
// given the regex and layout of the timestamp.  It will also optionally strip the outer date
// and detect if the remaining text contains a valid date
type DayMonthParser struct {
	dayMonthOptions
	datetimeRegex string
	layout        string
}

type dayMonthOptions struct {
	timezone       string
	year           string
	stripOuterDate bool
	innerDateRegex string
}

type dayMonthOption func(*dayMonthOptions)

func (d *dayMonthOptions) apply(opts ...dayMonthOption) {
	for _, opt := range opts {
		opt(d)
	}
}

func WithTimezone(timezone string) dayMonthOption {
	return func(d *dayMonthOptions) {
		d.timezone = timezone
	}
}

func WithYear(year string) dayMonthOption {
	return func(d *dayMonthOptions) {
		d.year = year
	}
}

func WithStripOuterDate() dayMonthOption {
	return func(d *dayMonthOptions) {
		d.stripOuterDate = true
	}
}

func WithInnerDateRegex(regex string) dayMonthOption {
	return func(d *dayMonthOptions) {
		d.innerDateRegex = regex
	}
}

func NewDayMonthParser(datetimeRegex string, layout string, opts ...dayMonthOption) DateParser {
	options := dayMonthOptions{
		timezone:       "UTC",
		year:           fmt.Sprint((time.Now().Year())),
		innerDateRegex: KlogRegex,
	}
	options.apply(opts...)
	return &DayMonthParser{
		dayMonthOptions: options,
		datetimeRegex:   datetimeRegex,
		layout:          layout,
	}
}

func (d *DayMonthParser) ParseTimestamp(log string) (time.Time, string, bool) {
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

	if d.stripOuterDate {
		cleaned := strings.TrimSpace(re.ReplaceAllString(log, ""))
		retLog = cleaned

		re = regexp.MustCompile(d.innerDateRegex)
		datestring = re.FindString(cleaned)
		valid = len(datestring) > 0
	}

	return datetime, retLog, valid
}
