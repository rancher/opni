package util

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
)

var ErrUnsupportedProtocolScheme = errors.New("unsupported protocol scheme")

func NewProtocolListener(addr string) (net.Listener, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "tcp", "tcp4":
		if u.Host == "" {
			return nil, fmt.Errorf("missing host in address %s", addr)
		}
		return net.Listen("tcp4", u.Host)
	case "unix":
		socketPath := u.Path
		if err := createSocketDir(socketPath); err != nil {
			return nil, err
		}
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				return nil, err
			}
		}
		return net.Listen("unix", socketPath)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProtocolScheme, u.Scheme)
	}
}

func createSocketDir(socketPath string) error {
	if _, err := os.Stat(filepath.Dir(socketPath)); os.IsNotExist(err) {
		return os.Mkdir(filepath.Dir(socketPath), 0700)
	}
	return nil
}
