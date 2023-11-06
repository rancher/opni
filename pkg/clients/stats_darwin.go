//go:build darwin

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
	Raw       unix.TCPConnectionInfo
}

// !! Darwin's sock_opt options don't have an option_name for accessing an underling tcp_stats struct
func (c *ConnStats) DeliveryRate() uint64 {
	return uint64(0)
}

func (c *ConnStats) RTT() time.Duration {
	return time.Duration(c.Raw.Rttcur) * time.Microsecond
}

func (c *ConnStats) HumanizedBytesReceived() string {
	str, err := util.Humanize(c.Raw.Rxbytes)
	if err != nil {
		return "0"
	}
	return str
}

func (c *ConnStats) HumanizedBytesSent() string {
	str, err := util.Humanize(c.Raw.Txbytes)
	if err != nil {
		return "0"
	}
	return str
}

// On darwin systems : <netinet/tpc.h>
const TCP_CONNECTION_INFO = 0x106

func queryStats(nc net.Conn) (ConnStats, error) {
	if nc == nil {
		return ConnStats{}, errors.New("not connected")
	}
	if scc, ok := nc.(syscall.Conn); ok {
		raw, err := scc.SyscallConn()
		if err != nil {
			return ConnStats{}, err
		}
		var info *unix.TCPConnectionInfo

		if err := raw.Control(func(fd uintptr) {
			info, err = unix.GetsockoptTCPConnectionInfo(int(fd), syscall.IPPROTO_TCP, TCP_CONNECTION_INFO)
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
