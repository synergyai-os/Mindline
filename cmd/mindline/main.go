package main

import (
	"os"

	"github.com/synergyai-os/Mindline/internal/cli"
)

func main() {
	runner := cli.NewRunner(cli.NewOSFileSystem())
	os.Exit(runner.Run(os.Args[1:], os.Stdout, os.Stderr))
}
