package pkp

import (
	"crypto/tls"
	"errors"
	"fmt"
)

var (
	ErrNoPins               = errors.New("no pins provided")
	ErrCertValidationFailed = errors.New("peer certificate validation failed")
)

func TLSConfig(pins []*PublicKeyPin, disablePins bool) (*tls.Config, error) {
	if len(pins) == 0 && !disablePins {
		return nil, ErrNoPins
	}
	copiedPins := make([]*PublicKeyPin, len(pins))
	for i, pin := range pins {
		if err := pin.Validate(); err != nil {
			if !disablePins {
				return nil, err
			}
		}
		copiedPins[i] = pin.DeepCopy()
	}

	/* #nosec G402 -- InsecureSkipVerify allowed in conjunction with VerifyConnection */
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if disablePins {
				return nil
			}
			peerCerts := cs.PeerCertificates
			// Validate the peer's certificate chain.
			for i := 0; i < len(peerCerts)-1; i++ {
				if err := peerCerts[i].CheckSignatureFrom(peerCerts[i+1]); err != nil {
					return fmt.Errorf("%w: %s", ErrCertValidationFailed, err)
				}
			}
			// Check each peer certificate for a matching pin.
			pinnedCert := -1
		CERTS:
			for i, peerCert := range peerCerts {
				for _, pin := range copiedPins {
					peerCertPin, _ := New(peerCert, pin.Algorithm)
					if pin.Equal(peerCertPin) {
						// Found a match
						pinnedCert = i
						break CERTS
					}
				}
			}
			if pinnedCert == -1 {
				return ErrCertValidationFailed
			}
			return nil
		},
	}, nil
}
