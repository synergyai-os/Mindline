//go:build linux

package operatorchannel

import (
	"os"
	"syscall"
)

func (AnonymousPipeVerifier) Verify(file *os.File) (*VerifiedReadChannel, error) {
	if file == nil {
		return nil, ErrUntrustedChannel
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 || info.Mode()&(os.ModeDevice|os.ModeCharDevice|os.ModeSocket|os.ModeSymlink) != 0 {
		return nil, ErrUntrustedChannel
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 0 {
		return nil, ErrUntrustedChannel
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, file.Fd(), syscall.F_GETFL, 0)
	if errno != 0 || flags&syscall.O_ACCMODE != syscall.O_RDONLY {
		return nil, ErrUntrustedChannel
	}
	_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, file.Fd(), syscall.F_SETFD, syscall.FD_CLOEXEC)
	if errno != 0 {
		return nil, ErrUntrustedChannel
	}
	return newVerifiedReadChannel(file), nil
}
