package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func (r Runner) runSlackRoute(args []string, stdout, stderr io.Writer) int {
	input, links, lenses, judgments, outDir, ok := parseSlackRouteArgs(args)
	if !ok {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if err := validatePrivateRuntimePaths(input, links, lenses, judgments, outDir); err != nil {
		fmt.Fprintf(stderr, "invalid private runtime path: %v\n", err)
		return ExitUsage
	}
	if err := r.validateDestinationOutDir(outDir); err != nil {
		fmt.Fprintf(stderr, "invalid routing output: %v\n", err)
		return ExitUsage
	}
	if err := privateio.PrepareDir(outDir); err != nil {
		fmt.Fprintf(stderr, "prepare routing output: %v\n", err)
		return ExitArtifactWrite
	}
	var payload slackadapter.Payload
	var artifacts routing.LinkArtifacts
	var profile routing.LensProfile
	var manifest routing.Judgments
	if err := readJSONFile(input, &payload); err != nil {
		fmt.Fprintf(stderr, "route Slack fixture: %v\n", err)
		return ExitProcess
	}
	if err := readJSONFile(links, &artifacts); err != nil {
		fmt.Fprintf(stderr, "route link artifacts: %v\n", err)
		return ExitProcess
	}
	if err := readJSONFile(lenses, &profile); err != nil {
		fmt.Fprintf(stderr, "route context lenses: %v\n", err)
		return ExitProcess
	}
	if err := readJSONFile(judgments, &manifest); err != nil {
		fmt.Fprintf(stderr, "route judgments: %v\n", err)
		return ExitProcess
	}
	result, err := slackadapter.CompileRouting(payload, artifacts, profile, manifest)
	if err != nil {
		fmt.Fprintf(stderr, "route Slack fixture: %v\n", err)
		return ExitProcess
	}
	if err := routing.Write(outDir, result); err != nil {
		fmt.Fprintf(stderr, "write routing artifacts: %v\n", err)
		return ExitArtifactWrite
	}
	return encodeJSON(stdout, stderr, result.Summary)
}

func parseSlackRouteArgs(args []string) (input, links, lenses, judgments, out string, ok bool) {
	if len(args) != 10 || args[0] != "route" || strings.TrimSpace(args[1]) == "" {
		return
	}
	input = args[1]
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", "", "", "", "", false
		}
		switch args[i] {
		case "--links":
			links = args[i+1]
		case "--lenses":
			lenses = args[i+1]
		case "--judgments":
			judgments = args[i+1]
		case "--out":
			out = args[i+1]
		default:
			return "", "", "", "", "", false
		}
	}
	return input, links, lenses, judgments, out, links != "" && lenses != "" && judgments != "" && out != ""
}

func validatePrivateRuntimePaths(paths ...string) error {
	return privateio.ValidateContained(strings.TrimSpace(os.Getenv("MINDLINE_PRIVATE_RUNTIME_ROOT")), paths...)
}
func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
func encodeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}
