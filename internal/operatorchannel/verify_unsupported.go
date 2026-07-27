//go:build !darwin && !linux

package operatorchannel

import "os"

func (AnonymousPipeVerifier) Verify(_ *os.File) (*VerifiedReadChannel, error) {
	return nil, ErrUntrustedChannel
}
