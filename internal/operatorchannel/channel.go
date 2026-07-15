package operatorchannel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
)

const (
	PairingFramePrefix = "MINDLINE_PAIR_V1 "
	MaximumFrameBytes  = 64
)

var pairingCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

var (
	ErrUntrustedChannel = errors.New("operator input is not a verified anonymous read pipe")
	ErrTerminalChannel  = errors.New("operator input channel is terminal")
	ErrMalformedFrame   = errors.New("operator pairing frame is malformed")
	ErrOversizeFrame    = errors.New("operator pairing frame exceeds 64 bytes")
)

type Verifier interface {
	Verify(*os.File) (*VerifiedReadChannel, error)
}

type AnonymousPipeVerifier struct{}

type VerifiedReadChannel struct {
	file      *os.File
	requests  chan pairingRequest
	cancels   chan uint64
	incoming  chan pairingResult
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
	nextID    atomic.Uint64
}

type pairingRequest struct {
	id       uint64
	response chan pairingResult
}

type pairingResult struct {
	code string
	err  error
}

func VerifyAnonymousReadPipe(file *os.File) (*VerifiedReadChannel, error) {
	return AnonymousPipeVerifier{}.Verify(file)
}

// ReadPairingCode reads the next frame. Valid frames leave the channel ready
// for lock/re-pair cycles; malformed input, oversize input, and EOF terminate it.
func (channel *VerifiedReadChannel) ReadPairingCode() (string, error) {
	return channel.ReadPairingCodeContext(context.Background())
}

// ReadPairingCodeContext registers a cancellable request with one persistent
// reader pump. An ordinary request expiry does not close or terminally poison
// the channel, so a later pairing request can read a new frame.
func (channel *VerifiedReadChannel) ReadPairingCodeContext(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if channel == nil || channel.file == nil || channel.requests == nil {
		return "", ErrUntrustedChannel
	}
	channel.startOnce.Do(func() {
		go channel.readPump()
		go channel.dispatch()
	})
	id := channel.nextID.Add(1)
	response := make(chan pairingResult, 1)
	request := pairingRequest{id: id, response: response}
	select {
	case channel.requests <- request:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-channel.closed:
		return "", ErrTerminalChannel
	}
	select {
	case result := <-response:
		return result.code, result.err
	case <-ctx.Done():
		select {
		case channel.cancels <- id:
		case <-channel.closed:
		}
		return "", ctx.Err()
	}
}

func newVerifiedReadChannel(file *os.File) *VerifiedReadChannel {
	return &VerifiedReadChannel{
		file: file, requests: make(chan pairingRequest), cancels: make(chan uint64),
		incoming: make(chan pairingResult), closed: make(chan struct{}),
	}
}

func (channel *VerifiedReadChannel) readPump() {
	reader := bufio.NewReaderSize(channel.file, MaximumFrameBytes+1)
	for {
		result := readFrame(reader)
		channel.incoming <- result
		if result.err != nil {
			return
		}
	}
}

func readFrame(reader *bufio.Reader) pairingResult {
	frame, err := reader.ReadString('\n')
	if len(frame) > MaximumFrameBytes || errors.Is(err, bufio.ErrBufferFull) {
		return pairingResult{err: ErrOversizeFrame}
	}
	if err != nil {
		if errors.Is(err, io.EOF) && len(frame) == 0 {
			return pairingResult{err: io.EOF}
		}
		return pairingResult{err: fmt.Errorf("%w: %v", ErrMalformedFrame, err)}
	}
	if len(frame) != len(PairingFramePrefix)+22+1 || frame[:len(PairingFramePrefix)] != PairingFramePrefix || frame[len(frame)-1] != '\n' {
		return pairingResult{err: ErrMalformedFrame}
	}
	code := frame[len(PairingFramePrefix) : len(frame)-1]
	if !pairingCodePattern.MatchString(code) {
		return pairingResult{err: ErrMalformedFrame}
	}
	return pairingResult{code: code}
}

func (channel *VerifiedReadChannel) dispatch() {
	pending := make([]pairingRequest, 0, 1)
	queued := make([]pairingResult, 0, 2)
	var terminal error
	discardUntilRequest := false
	for {
		select {
		case request := <-channel.requests:
			discardUntilRequest = false
			if len(queued) != 0 {
				request.response <- queued[0]
				queued = queued[1:]
				if terminal != nil && len(queued) == 0 {
					channel.terminate()
					return
				}
			} else if terminal != nil {
				request.response <- pairingResult{err: terminal}
			} else {
				pending = append(pending, request)
			}
		case id := <-channel.cancels:
			for index := range pending {
				if pending[index].id == id {
					pending = append(pending[:index], pending[index+1:]...)
					break
				}
			}
			discardUntilRequest = true
			queued = nil
		case result := <-channel.incoming:
			if result.err != nil {
				terminal = result.err
				for len(pending) != 0 && len(queued) != 0 {
					pending[0].response <- queued[0]
					pending = pending[1:]
					queued = queued[1:]
				}
				for _, request := range pending {
					request.response <- result
				}
				pending = nil
				if len(queued) == 0 {
					channel.terminate()
					return
				}
				continue
			}
			if len(pending) != 0 {
				request := pending[0]
				pending = pending[1:]
				request.response <- result
			} else if !discardUntilRequest && len(queued) < 16 {
				queued = append(queued, result)
			}
			// After cancellation, frames are discarded until a new request is
			// registered; they cannot become authority for a later generation.
		}
	}
}

func (channel *VerifiedReadChannel) close() error {
	if channel == nil || channel.file == nil {
		return nil
	}
	var err error
	channel.closeOnce.Do(func() {
		err = channel.file.Close()
		close(channel.closed)
	})
	return err
}

func (channel *VerifiedReadChannel) terminate() {
	channel.closeOnce.Do(func() {
		_ = channel.file.Close()
		close(channel.closed)
	})
}
