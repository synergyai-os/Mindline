package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/synergyai-os/Mindline/internal/operatorchannel"
)

func main() {
	if len(os.Args) != 2 || !filepath.IsAbs(os.Args[1]) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: mindline-operator-launcher <absolute-mindline-executable>")
		os.Exit(2)
	}
	verified, err := operatorchannel.VerifyAnonymousReadPipe(os.Stdin)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "mindline-operator-launcher:", err)
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := (operatorchannel.ProcessLauncher{}).Run(os.Args[1], verified, nil, os.Stdout, os.Stderr, signals); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "mindline-operator-launcher:", err)
		os.Exit(1)
	}
}
