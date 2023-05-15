package fingerprint

import (
	"strconv"
	"time"
)

type Fingerprint string

func Default() Fingerprint {
	return Fingerprint(strconv.Itoa(int(time.Now().Unix())))
}
