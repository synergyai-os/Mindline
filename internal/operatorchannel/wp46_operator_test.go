package operatorchannel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	firstPairingCode  = "abcdefghijklmnopqrstuv"
	secondPairingCode = "ZYXWVUTSRQPONMLKJIHGFE"
)

func TestWP46_OperatorChannelVerifierRejectsUntrustedInputs(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	verified, err := VerifyAnonymousReadPipe(reader)
	if err != nil {
		t.Fatalf("anonymous read pipe rejected: %v", err)
	}
	defer verified.close()
	if _, err := VerifyAnonymousReadPipe(writer); !errors.Is(err, ErrUntrustedChannel) {
		t.Fatalf("pipe writer accepted: %v", err)
	}

	file, err := os.CreateTemp(t.TempDir(), "operator-file")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := VerifyAnonymousReadPipe(file); !errors.Is(err, ErrUntrustedChannel) {
		t.Fatalf("regular file accepted: %v", err)
	}
	device, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if _, err := VerifyAnonymousReadPipe(device); !errors.Is(err, ErrUntrustedChannel) {
		t.Fatalf("device accepted: %v", err)
	}

	if runtime.GOOS != "windows" {
		fifo := filepath.Join(t.TempDir(), "named-fifo")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		named, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer named.Close()
		if _, err := VerifyAnonymousReadPipe(named); !errors.Is(err, ErrUntrustedChannel) {
			t.Fatalf("named FIFO accepted: %v", err)
		}
	}

	placeholder, err := os.CreateTemp("/tmp", "ml-op-")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := placeholder.Name()
	_ = placeholder.Close()
	_ = os.Remove(socketPath)
	defer os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	socketFile, err := listener.(*net.UnixListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer socketFile.Close()
	if _, err := VerifyAnonymousReadPipe(socketFile); !errors.Is(err, ErrUntrustedChannel) {
		t.Fatalf("socket accepted: %v", err)
	}
}

func TestWP46_UnrelatedSameUIDProcessCannotInjectOperatorConfirmation(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyAnonymousReadPipe(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer verified.close()
	info, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 0 {
		t.Fatal("verified operator pipe unexpectedly has a filesystem link")
	}
	if name := reader.Name(); name != "|0" {
		t.Fatalf("anonymous pipe exposed a reusable path: %q", name)
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), reader.Name())); !os.IsNotExist(err) {
		t.Fatalf("anonymous input became path-addressable: %v", err)
	}
	_, _ = io.WriteString(writer, PairingFramePrefix+firstPairingCode+"\n")
	code, err := verified.ReadPairingCode()
	if err != nil || code != firstPairingCode {
		t.Fatalf("verified writer could not confirm: code=%q err=%v", code, err)
	}
}

func TestWP46_OperatorChannelSupportsRepairAndExpiryWithoutReaderLeak(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyAnonymousReadPipe(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer verified.close()
	defer writer.Close()
	for _, expected := range []string{firstPairingCode, secondPairingCode} {
		result := make(chan struct {
			code string
			err  error
		}, 1)
		go func() {
			code, readErr := verified.ReadPairingCode()
			result <- struct {
				code string
				err  error
			}{code, readErr}
		}()
		time.Sleep(time.Millisecond)
		_, _ = io.WriteString(writer, PairingFramePrefix+expected+"\n")
		read := <-result
		code, err := read.code, read.err
		if err != nil || code != expected {
			t.Fatalf("multi-frame read = %q, %v", code, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := verified.ReadPairingCodeContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pairing expiry = %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, readErr := verified.ReadPairingCodeContext(ctx)
		result <- struct {
			code string
			err  error
		}{code, readErr}
	}()
	time.Sleep(time.Millisecond)
	_, _ = io.WriteString(writer, PairingFramePrefix+firstPairingCode+"\n")
	read := <-result
	code, err := read.code, read.err
	if err != nil || code != firstPairingCode {
		t.Fatalf("channel did not recover after expiry: %q, %v", code, err)
	}
}

func TestWP46_OperatorLauncherOwnsWriterAndChildCannotInheritIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	server := filepath.Join(t.TempDir(), "server-fixture")
	script := []byte("#!/bin/sh\nread first || exit 11\nread second || exit 12\nif read third; then exit 13; fi\nprintf '%s\\n%s\\n%s\\n' \"$1 $2\" \"$first\" \"$second\"\n")
	if err := os.WriteFile(server, script, 0o700); err != nil {
		t.Fatal(err)
	}
	operatorReader, operatorWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyAnonymousReadPipe(operatorReader)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	done := make(chan error, 1)
	signals := make(chan os.Signal, 1)
	go func() { done <- (ProcessLauncher{}).Run(server, verified, nil, &stdout, &stdout, signals) }()
	time.Sleep(10 * time.Millisecond)
	_, _ = io.WriteString(operatorWriter, PairingFramePrefix+firstPairingCode+"\n"+PairingFramePrefix+secondPairingCode+"\n")
	_ = operatorWriter.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		signals <- os.Interrupt
		t.Fatal("child retained the launcher-owned pipe writer")
	}
	want := "activation serve\n" + PairingFramePrefix + firstPairingCode + "\n" + PairingFramePrefix + secondPairingCode + "\n"
	if got := stdout.String(); got != want {
		t.Fatalf("child argv/input = %q, want %q", got, want)
	}
	if strings.Contains(stdout.String(), "open ") {
		t.Fatal("launcher invoked a browser opener")
	}
}

func TestWP46_OperatorLauncherControllerEOFStopsChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	server := filepath.Join(t.TempDir(), "server-fixture")
	if err := os.WriteFile(server, []byte("#!/bin/sh\nwhile read line; do :; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	operatorReader, operatorWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer operatorWriter.Close()
	operator, err := VerifyAnonymousReadPipe(operatorReader)
	if err != nil {
		t.Fatal(err)
	}
	controllerReader, controllerWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	controller, err := VerifyAnonymousReadPipe(controllerReader)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- (ProcessLauncher{}).Run(server, operator, controller, io.Discard, io.Discard, nil) }()
	time.Sleep(10 * time.Millisecond)
	if err := controllerWriter.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrControllerClosed) {
			t.Fatalf("controller EOF result = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("controller EOF did not stop the child")
	}
}
