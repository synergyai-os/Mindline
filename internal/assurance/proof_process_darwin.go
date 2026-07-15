//go:build darwin

package assurance

import (
	"bytes"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

type observedCommandResult struct {
	Stdout        string
	Stderr        string
	ChildEvents   int
	BrowserEvents int
}

func runObservedCommand(path string, args, environment []string, directory string) (observedCommandResult, error) {
	command := exec.Command(path, args...)
	command.Dir = directory
	command.Env = environment
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return observedCommandResult{}, err
	}
	kqueue, err := syscall.Kqueue()
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return observedCommandResult{}, err
	}
	defer syscall.Close(kqueue)
	change := syscall.Kevent_t{Ident: uint64(command.Process.Pid), Filter: syscall.EVFILT_PROC, Flags: syscall.EV_ADD | syscall.EV_ENABLE | syscall.EV_CLEAR, Fflags: syscall.NOTE_FORK | syscall.NOTE_EXEC | syscall.NOTE_EXIT}
	if _, err := syscall.Kevent(kqueue, []syscall.Kevent_t{change}, nil, nil); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return observedCommandResult{}, errors.New("process observer could not bind the baseline command")
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	childEvents := 0
	for {
		select {
		case waitErr := <-wait:
			result := observedCommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ChildEvents: childEvents}
			if waitErr != nil {
				return result, waitErr
			}
			return result, nil
		default:
			events := make([]syscall.Kevent_t, 8)
			timeout := syscall.NsecToTimespec((10 * time.Millisecond).Nanoseconds())
			count, eventErr := syscall.Kevent(kqueue, nil, events, &timeout)
			if eventErr != nil && !errors.Is(eventErr, syscall.EINTR) {
				_ = command.Process.Kill()
				<-wait
				return observedCommandResult{}, eventErr
			}
			for _, event := range events[:count] {
				if event.Fflags&syscall.NOTE_FORK != 0 {
					childEvents++
				}
			}
		}
	}
}

func stopProcessGroup(pgid int, crash bool, deadline time.Duration) error {
	if pgid <= 0 {
		return errors.New("invalid proof process group")
	}
	signal := syscall.SIGTERM
	if crash {
		signal = syscall.SIGKILL
	}
	if err := syscall.Kill(-pgid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		err := syscall.Kill(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !crash {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	for index := 0; index < 50; index++ {
		if errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("proof process group did not stop")
}
