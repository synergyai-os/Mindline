package main

import (
	"os"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

func main() {
	os.Exit(assurance.RunProofRunner(os.Args[1:], os.Stdout, os.Stderr))
}
