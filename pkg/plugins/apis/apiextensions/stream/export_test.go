package stream

import "time"

func X_SetDiscoveryTimeout(timeout time.Duration) time.Duration {
	return discoveryTimeout.Swap(timeout)
}
