package alerting

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

var (
	AlertPath            = "/var/lib/alerting"
	AlertLogPrefix       = "alertlog"
	TimeFormat           = "2006-02-01"
	Separator            = "_"
	BucketMaxSize  int64 = 1024 * 2 // 2KB

)

type BucketInfo struct {
	ConditionId string
	Timestamp   string
	Number      int
}

func Parse(fileName string) (string, string) {
	parts := strings.Split(fileName, Separator)
	if len(parts) == 1 {
		return parts[0], "0"
	}
	return parts[0], parts[1]
}

// returns the indices created by the alert logger
// an index is a directory containing all the logs for a specific condition
func GetIndices() ([]*BucketInfo, error) {
	dir := []string{}
	res := []*BucketInfo{}
	things, err := ioutil.ReadDir(AlertPath)
	if err != nil {
		return nil, err
	}
	for _, thing := range things {
		if thing.IsDir() {
			dir = append(dir, thing.Name())
		}
	}
	for _, d := range dir {
		res = append(res, &BucketInfo{
			ConditionId: d,
		})
	}
	for _, b := range res {
		b.MostRecent()
	}
	return res, nil
}

func (b *BucketInfo) Construct() string {
	return path.Join(AlertPath, b.ConditionId, (b.Timestamp + Separator + strconv.Itoa(b.Number)))
}

func (b *BucketInfo) MostRecent() error {
	files, err := ioutil.ReadDir(path.Join(AlertPath, b.ConditionId))
	if err != nil {
		return err
	}
	timeStampToNumber := make(map[time.Time]int)
	for _, file := range files {
		timestamp, number := Parse(file.Name())
		n, err := strconv.Atoi(number)
		if err != nil {
			n = 0
		}
		t, err := time.Parse(TimeFormat, timestamp)
		if err != nil {
			return err
		}
		if _, ok := timeStampToNumber[t]; ok {
			if n > timeStampToNumber[t] {
				timeStampToNumber[t] = n
			}
		} else {
			timeStampToNumber[t] = n
		}
	}

	keys := make([]time.Time, 0, len(timeStampToNumber))
	for t := range timeStampToNumber {
		keys = append(keys, t)
	}
	if len(keys) > 0 {
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].Before(keys[j])
		})
		b.Timestamp = keys[len(keys)-1].Format(TimeFormat)
		b.Number = timeStampToNumber[keys[len(keys)-1]]
	}
	return nil
}

func (b *BucketInfo) Size() (int64, error) {
	fi, err := os.Stat(b.Construct())
	if err != nil {
		return -1, err
	}
	return fi.Size(), nil
}

func (b *BucketInfo) IsFull() bool {
	size, err := b.Size()
	if err != nil {
		return false
	}
	return size >= BucketMaxSize
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (b *BucketInfo) GenerateNewBucket() error {
	if b.ConditionId == "" {
		return fmt.Errorf("GenerateBucket : Failed precondition, should have a set ConditionId field")
	}
	timeStr := time.Now().Format(TimeFormat)
	b.Timestamp = timeStr
	b.Number = 0
	for fileExists(b.Construct()) {
		b.Number += 1
	}
	return nil
}

func (b *BucketInfo) Create() error {
	err := CreateIndex(b.ConditionId)
	if err != nil {
		return err
	}
	err = b.GenerateNewBucket()
	if err != nil {
		return err
	}
	f, err := os.Create(b.Construct())
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

func (b *BucketInfo) Append(log *corev1.AlertLog) error {
	b.MostRecent() // update the timestamp and number
	cur := time.Now()
	timeStr := cur.Format(TimeFormat)
	cur, _ = time.Parse(TimeFormat, timeStr)
	prev, err := time.Parse(TimeFormat, b.Timestamp)
	if err != nil {
		b.GenerateNewBucket()
	} else if prev.Before(cur) {
		b.GenerateNewBucket()
	}
	if b.IsFull() {
		b.GenerateNewBucket()
	}
	f, err := os.OpenFile(b.Construct(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(log.String())
	if err != nil {
		return err
	}
	return nil
}

func CreateIndex(conditionID string) error {
	return os.Mkdir(path.Join(AlertPath, conditionID), 0777)
}
