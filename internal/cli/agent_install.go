package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/synergyai-os/Mindline/internal/localservice"
)

func (r Runner) runAgentInstall(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 ||
		!onlyAgentKeys(options.values, "runtime-root", "memory-root", "start", "source-binary") {
		return agentUsage(stderr)
	}
	config, err := localservice.DefaultConfig()
	if err != nil {
		return agentFailure(stderr, err)
	}
	if options.values["runtime-root"] != "" || options.values["memory-root"] != "" {
		runtimeRoot := config.RuntimeRoot
		memoryRoot := config.MemoryRoot
		if options.values["runtime-root"] != "" {
			runtimeRoot = options.values["runtime-root"]
		}
		if options.values["memory-root"] != "" {
			memoryRoot = options.values["memory-root"]
		}
		config, err = localservice.ConfigFromRoots(runtimeRoot, memoryRoot)
		if err != nil {
			return agentFailure(stderr, err)
		}
	}
	start := true
	if value := options.values["start"]; value != "" {
		start, err = strconv.ParseBool(value)
		if err != nil {
			return agentUsage(stderr)
		}
	}
	sourceBinary := options.values["source-binary"]
	if sourceBinary == "" {
		sourceBinary, err = os.Executable()
		if err != nil {
			return agentFailure(stderr, err)
		}
	}
	receipt, err := localservice.Install(localservice.InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: sourceBinary, Start: start,
	})
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runAgentRestart(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	receipt, err := localservice.Restart(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runAgentUninstall(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	receipt, err := localservice.Uninstall(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	fmt.Fprintln(stderr, "Mindline agent service removed; private evidence and relevance state were preserved.")
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}
