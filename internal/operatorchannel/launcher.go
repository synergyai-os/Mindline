package operatorchannel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var (
	ErrControllerClosed = errors.New("proof controller liveness channel closed")
)

type Launcher interface {
	Run(serverExecutable string, operator, optionalController *VerifiedReadChannel, stdout, stderr io.Writer, signals <-chan os.Signal) error
}

type ProcessLauncher struct{}

func (ProcessLauncher) Run(serverExecutable string, operator, optionalController *VerifiedReadChannel, stdout, stderr io.Writer, signals <-chan os.Signal) error {
	if operator == nil || operator.file == nil {
		return ErrUntrustedChannel
	}
	if optionalController != nil && optionalController.file == nil {
		return ErrUntrustedChannel
	}
	if !filepath.IsAbs(serverExecutable) {
		return errors.New("server executable must be absolute")
	}
	info, err := os.Lstat(serverExecutable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		return errors.New("server executable must be a regular non-symlink executable")
	}
	serverInput, launcherWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer serverInput.Close()
	defer launcherWriter.Close()
	command := exec.Command(serverExecutable, "activation", "serve")
	command.Stdin = serverInput
	command.Stdout = stdout
	command.Stderr = stderr
	if optionalController != nil {
		// fd 3 is a read-only duplicate of the controller-liveness pipe. The
		// writer remains exclusively in the proof controller, never this
		// launcher, the server, or descendants.
		command.ExtraFiles = []*os.File{optionalController.file}
	}
	if err := command.Start(); err != nil {
		return err
	}
	// The child owns the only reader. The launcher owns the only writer, and the
	// writer is not in ExtraFiles, so neither server nor descendants inherit it.
	if err := serverInput.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}

	type event struct {
		kind string
		err  error
	}
	events := make(chan event, 4)
	done := make(chan struct{})
	defer close(done)
	var closeOnce sync.Once
	closeWriter := func() { closeOnce.Do(func() { _ = launcherWriter.Close() }) }
	go func() {
		for {
			code, readErr := operator.ReadPairingCode()
			if readErr == nil {
				_, readErr = io.WriteString(launcherWriter, PairingFramePrefix+code+"\n")
			}
			if readErr != nil {
				closeWriter()
				events <- event{"operator", readErr}
				return
			}
		}
	}()
	if optionalController != nil {
		go func() {
			buffer := make([]byte, 1)
			count, readErr := optionalController.file.Read(buffer)
			if readErr == nil || count != 0 {
				readErr = errors.New("proof controller liveness pipe carried payload")
			}
			events <- event{"controller", readErr}
		}()
	}
	if signals != nil {
		go func() {
			select {
			case signal, ok := <-signals:
				if ok {
					events <- event{"signal", fmt.Errorf("launcher received %s", signal)}
				}
			case <-done:
			}
		}()
	}
	go func() { events <- event{"child", command.Wait()} }()

	for {
		event := <-events
		switch event.kind {
		case "child":
			closeWriter()
			_ = operator.close()
			if optionalController != nil {
				_ = optionalController.close()
			}
			return event.err
		case "operator":
			if event.err != nil {
				if !errors.Is(event.err, io.EOF) && !errors.Is(event.err, ErrTerminalChannel) {
					_ = command.Process.Kill()
				}
				continue
			}
		case "controller":
			_ = command.Process.Kill()
			// Wait owns process reaping; its event is consumed on the next loop.
			for {
				finished := <-events
				if finished.kind == "child" {
					return ErrControllerClosed
				}
			}
		case "signal":
			_ = command.Process.Signal(os.Interrupt)
		}
	}
}
