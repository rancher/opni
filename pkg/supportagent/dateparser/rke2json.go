package dateparser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	EtcdTimestampRegex  = `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{6}`
	EtcdTimestampLayout = "2006-01-02 15:04:05.999999Z07:00"
	RFC3339Milli        = "2006-01-02T15:04:05.999Z07:00"
)

// RKE2EtcdParser will parse etcd logs for RKE2.  These logs can either be in json or plain text.
type RKE2EtcdParser struct{}

type EtcdJSONLog struct {
	LogLevel  string `json:"level,omitempty"`
	Timestamp string `json:"ts,omitempty"`
	Message   string `json:"msg,omitempty"`
}

func (r RKE2EtcdParser) ParseTimestamp(log string) (time.Time, string, bool) {
	if strings.HasPrefix(log, "{") {
		jsonLog := &EtcdJSONLog{}
		json.Unmarshal([]byte(log), jsonLog)
		datetime, err := time.Parse(RFC3339Milli, jsonLog.Timestamp)
		if err != nil {
			panic(err)
		}
		return datetime, log, true
	}
	re := regexp.MustCompile(EtcdTimestampRegex)
	datestring := re.FindString(log)
	if len(datestring) == 0 {
		return time.Now(), log, false
	}
	datetime, err := time.Parse(EtcdTimestampLayout, fmt.Sprintf("%sZ", datestring))
	if err != nil {
		panic(err)
	}
	return datetime, log, true
}
