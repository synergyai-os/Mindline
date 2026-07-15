package controlui

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"

	"github.com/synergyai-os/Mindline/internal/operatorchannel"
)

type operatorPairingConfirmer struct {
	channel *operatorchannel.VerifiedReadChannel
}

// NewOperatorPairingConfirmer adapts the verified launcher-owned anonymous
// pipe to the browser pairing port. The browser challenge remains
// non-authorizing until the exact canonical bytes arrive through that pipe.
func NewOperatorPairingConfirmer(channel *operatorchannel.VerifiedReadChannel) PairingConfirmer {
	return &operatorPairingConfirmer{channel: channel}
}

func (confirmer *operatorPairingConfirmer) ConfirmPairing(ctx context.Context, challenge string) error {
	if confirmer == nil || confirmer.channel == nil {
		return ErrPairingInputMalformed
	}
	code, err := confirmer.channel.ReadPairingCodeContext(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, operatorchannel.ErrMalformedFrame) ||
			errors.Is(err, operatorchannel.ErrOversizeFrame) ||
			errors.Is(err, operatorchannel.ErrTerminalChannel) ||
			errors.Is(err, io.EOF) {
			return ErrPairingInputMalformed
		}
		return err
	}
	if len(code) != len(challenge) || subtle.ConstantTimeCompare([]byte(code), []byte(challenge)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

var _ PairingConfirmer = (*operatorPairingConfirmer)(nil)
