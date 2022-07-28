package alerting

import (
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/pkg/alerting/shared"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	AlertPath            = "/var/lib/alerting"
	AlertLogPrefix       = "alertlog"
	TimeFormat           = "2006-02-01"
	Separator            = "_"
	BucketMaxSize  int64 = 1024 * 2 // 2KB

)

type BucketInfo struct {
	Path      string
	Timestamp string
	Index     int
}

// returns the buckets created by the alerting logger
func GetBuckets(lg hclog.Logger) ([]string, error) {
	res := []string{}
	files, err := ioutil.ReadDir(AlertPath)
	if err != nil {
		lg.Error("failed to read alerting buckets from disk", "error", err)
		return nil, err
	}

	for _, file := range files {
		res = append(res, AlertPath+"/"+file.Name())
	}
	return res, nil
}

// gets the size of the bucket in bytes
func BucketSize(bucketPath string) (int64, error) {
	fi, err := os.Stat(bucketPath)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func BucketIsFull(bucketPath string) bool {
	size, err := BucketSize(bucketPath)
	if err != nil {
		return false
	}
	return size >= BucketMaxSize
}

// returns nil if the file doesn't exist
func checkFileExists(filePath string) error {
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return nil
	}
	return fmt.Errorf("file exists")
}

func GenerateBucketName() (string, error) {
	now := time.Now()
	prefix := AlertLogPrefix
	suffix := now.Format(TimeFormat)
	prev := fmt.Sprintf("%s%s%s", prefix, Separator, suffix)
	cur := prev
	index := 1
	for checkFileExists(AlertPath+"/"+cur) != nil {
		cur = prev + fmt.Sprintf("%s%d", Separator, index)
		index += 1
	}

	return cur, nil
}

func CreateBucket() error {
	name, err := GenerateBucketName()
	if err != nil {
		return err
	}
	f, err := os.Create(AlertPath + "/" + name)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

func ParseNameToInfo(bucketName string, lg hclog.Logger) (*BucketInfo, error) {
	parts := strings.Split(bucketName, Separator)
	if len(parts) == 2 {
		return &BucketInfo{
			Path:      parts[0],
			Timestamp: parts[1],
		}, nil
	} else if len(parts) == 3 {
		index, err := strconv.Atoi(parts[2])
		if err != nil {
			lg.With("bucket index", parts[2]).Error(
				shared.AlertingErrBucketIndexInvalid.Error(),
			)
			return nil, shared.AlertingErrBucketIndexInvalid // internal error
		}
		return &BucketInfo{
			Path:      parts[0],
			Timestamp: parts[1],
			Index:     index,
		}, nil
	}
	lg.Error(
		fmt.Sprintf("Fatal : %s", shared.AlertingErrParseBucket.Error()),
	)
	return nil, shared.AlertingErrParseBucket // internal error
}

// Sort buckets, sorts buckets by timestamp, then by index
func SortBuckets(buckets []BucketInfo) ([]BucketInfo, error) {
	// TODO :
	return nil, nil
}
