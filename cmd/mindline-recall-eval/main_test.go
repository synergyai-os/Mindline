package main

import (
	"bytes"
	"testing"
)

func TestRejectsUnknownOrUnboundedInvocation(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"seal", "socket", "out"}, bytes.NewBufferString(`{"unknown":true}`), &output); err == nil {
		t.Fatal("unknown draft fields were accepted")
	}
	if err := run([]string{"compare"}, bytes.NewReader(nil), &output); err == nil {
		t.Fatal("invalid invocation was accepted")
	}
}
