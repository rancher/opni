//go:build unix

package clients

import (
	"net"
	"syscall"
	"time"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/util"
	"golang.org/x/sys/unix"
)

func (gc *gatewayClient) QueryConnStats() (ConnStats, error) {
	gc.ncMu.Lock()
	defer gc.ncMu.Unlock()
	return queryStats(gc.nc)
}

type ConnStats struct {
	Timestamp time.Time
	Raw       unix.TCPInfo
}

// Returns the socket throughput in bytes per second
func (c *ConnStats) DeliveryRate() uint64 {
	return c.Raw.Delivery_rate
}

func (c *ConnStats) RTT() time.Duration {
	return time.Duration(c.Raw.Rtt) * time.Microsecond
}

func (c *ConnStats) HumanizedBytesReceived() string {
	str, err := util.Humanize(c.Raw.Bytes_received)
	if err != nil {
		return "0"
	}
	return str
}

func (c *ConnStats) HumanizedBytesSent() string {
	str, err := util.Humanize(c.Raw.Bytes_sent)
	if err != nil {
		return "0"
	}
	return str
}

func queryStats(nc net.Conn) (ConnStats, error) {
	if nc == nil {
		return ConnStats{}, errors.New("not connected")
	}
	if scc, ok := nc.(syscall.Conn); ok {
		raw, err := scc.SyscallConn()
		if err != nil {
			return ConnStats{}, err
		}
		var info *unix.TCPInfo
		if err := raw.Control(func(fd uintptr) {
			info, err = unix.GetsockoptTCPInfo(int(fd), syscall.IPPROTO_TCP, syscall.TCP_INFO)
		}); err != nil {
			return ConnStats{}, err
		}
		if err != nil {
			return ConnStats{}, err
		}
		return ConnStats{
			Timestamp: time.Now(),
			Raw:       *info,
		}, nil
	}
	return ConnStats{}, errors.New("underlying connection is not a syscall.Conn")
}
