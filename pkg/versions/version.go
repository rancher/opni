package versions

import "sync"

var (
	Version   string
	versionMu = sync.RWMutex{}
)

func SetVersionForTest(v string) {
	versionMu.Lock()
	Version = v
}

func UnlockVersionForTest() {
	versionMu.Unlock()
}
