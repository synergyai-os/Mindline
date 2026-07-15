package controlui

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/synergyai-os/Mindline/internal/operatorchannel"
)

func TestWP46OperatorPairingConfirmsExactCodeAndAllowsRepair(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	verified, err := operatorchannel.VerifyAnonymousReadPipe(reader)
	if err != nil {
		t.Fatal(err)
	}
	confirmer := NewOperatorPairingConfirmer(verified)
	first := "abcdefghijklmnopqrstuv"
	second := "ABCDEFGHIJKLMNOPQRSTUV"
	go func() {
		_, _ = writer.Write([]byte(operatorchannel.PairingFramePrefix + first + "\n"))
		_, _ = writer.Write([]byte(operatorchannel.PairingFramePrefix + second + "\n"))
	}()
	if err := confirmer.ConfirmPairing(context.Background(), second); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong exact challenge was not rejected: %v", err)
	}
	if err := confirmer.ConfirmPairing(context.Background(), second); err != nil {
		t.Fatalf("valid repair frame was not accepted: %v", err)
	}
}

func TestWP46OperatorPairingMapsMalformedChannelToPermanentSentinel(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	verified, err := operatorchannel.VerifyAnonymousReadPipe(reader)
	if err != nil {
		t.Fatal(err)
	}
	confirmer := NewOperatorPairingConfirmer(verified)
	go func() { _, _ = writer.Write([]byte("malformed\n")) }()
	if err := confirmer.ConfirmPairing(context.Background(), "abcdefghijklmnopqrstuv"); !errors.Is(err, ErrPairingInputMalformed) {
		t.Fatalf("malformed operator frame did not close the pairing authority: %v", err)
	}
}
