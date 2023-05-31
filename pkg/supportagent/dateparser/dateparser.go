package dateparser

import "time"

const (
	KlogRegex     = `\d{4} \d{2}:\d{2}:\d{2}.\d{6}`
	EtcdRegex     = `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{6}`
	RancherRegex  = `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}`
	JournaldRegex = `^[A-Z][a-z]{2} \d{1,2} \d{2}:\d{2}:\d{2}`

	RancherLayout  = "2006/01/02 15:05:05"
	KlogLayout     = "0102 15:04:05.999999 MST 2006"
	JournaldLayout = "Jan 02 15:04:05 MST 2006"
)

type LogType string

const (
	LogTypeControlplane LogType = "controlplane"
	LogTypeRancher      LogType = "rancher"
)

// DateParser is an interface for parsing the timestamp from a log line
// It returns the datetime, the log line, and a boolean indicating if the datetime was found
type DateParser interface {
	ParseTimestamp(log string) (time.Time, string, bool) // Parse timestamp should have the implementation for parsing the timestamp from a log line
}
