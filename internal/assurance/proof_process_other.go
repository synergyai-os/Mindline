//go:build !darwin

package assurance

import (
	"errors"
	"time"
)

type observedCommandResult struct {
	Stdout, Stderr             string
	ChildEvents, BrowserEvents int
}

func runObservedCommand(_ string, _, _ []string, _ string) (observedCommandResult, error) {
	return observedCommandResult{}, errors.New("signed baseline process observation requires Darwin kqueue")
}

func stopProcessGroup(_ int, _ bool, _ time.Duration) error {
	return errors.New("signed proof process containment requires Darwin")
}
