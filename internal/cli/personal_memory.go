package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const maximumNativeBatchBytes = 32 << 20

func (r Runner) runPersonalMemory(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "import-slack":
		return r.runPersonalMemoryImportSlack(args[1:], stdout, stderr)
	case "enrich":
		return r.runPersonalMemoryEnrich(args[1:], stdout, stderr)
	case "search":
		return r.runPersonalMemorySearch(args[1:], stdout, stderr)
	case "lenses":
		return r.runPersonalMemoryLenses(args[1:], stdout, stderr)
	case "get":
		return r.runPersonalMemoryGet(args[1:], stdout, stderr)
	case "status":
		return r.runPersonalMemoryStatus(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
}

func (r Runner) runPersonalMemoryImportSlack(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	var input []byte
	if positionals[0] == "-" {
		input, err = io.ReadAll(io.LimitReader(r.nativeInput, maximumNativeBatchBytes+1))
		if err == nil && len(input) > maximumNativeBatchBytes {
			err = fmt.Errorf("native batch exceeds size limit")
		}
	} else {
		input, err = r.fs.ReadFileBounded(positionals[0], maximumNativeBatchBytes)
	}
	if err != nil {
		fmt.Fprintln(stderr, "read native Slack batch: source unavailable")
		return ExitUsage
	}
	var batch acquisitionslack.NativeBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil {
		fmt.Fprintln(stderr, "read native Slack batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	captureBatch, err := slackadapter.CaptureBatchFromNative(batch)
	if err != nil {
		fmt.Fprintf(stderr, "import personal evidence: %v\n", err)
		return ExitProcess
	}
	receipt, err := repository.Import(captureBatch)
	if err != nil {
		fmt.Fprintf(stderr, "import personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runPersonalMemoryEnrich(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	input, err := r.readPersonalMemoryInput(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "read personal enrichment batch: source unavailable")
		return ExitUsage
	}
	var batch personalmemory.EnrichmentBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil ||
		batch.SchemaVersion != personalmemory.EnrichmentBatchSchemaVersion {
		fmt.Fprintln(stderr, "read personal enrichment batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	receipt, err := repository.MergeEnrichment(batch)
	if err != nil {
		fmt.Fprintf(stderr, "merge personal evidence enrichment: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runPersonalMemorySearch(args []string, stdout, stderr io.Writer) int {
	positionals, root, limit, err := parsePersonalMemoryArgs(args, true)
	if err != nil || len(positionals) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	packet, err := personalmemory.NewLexicalRetriever(repository).Search(personalmemory.SearchRequest{
		Query: strings.Join(positionals, " "), Limit: limit,
	})
	if err != nil {
		fmt.Fprintf(stderr, "search personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, packet)
}

func (r Runner) runPersonalMemoryLenses(args []string, stdout, stderr io.Writer) int {
	positionals, root, limit, err := parsePersonalMemoryArgs(args, true)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	input, err := r.readPersonalMemoryInput(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "read personal lens batch: source unavailable")
		return ExitUsage
	}
	var batch personalmemory.LensBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil {
		fmt.Fprintln(stderr, "read personal lens batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	packet, err := personalmemory.NewLexicalRetriever(repository).ReviewLenses(batch, limit)
	if err != nil {
		fmt.Fprintf(stderr, "review personal evidence lenses: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, packet)
}

func (r Runner) runPersonalMemoryGet(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	record, err := personalmemory.NewLexicalRetriever(repository).Get(positionals[0])
	if err != nil {
		fmt.Fprintf(stderr, "get personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, record)
}

func (r Runner) runPersonalMemoryStatus(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	status, err := repository.Status()
	if err != nil {
		fmt.Fprintf(stderr, "read personal evidence status: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, status)
}

func parsePersonalMemoryArgs(args []string, allowLimit bool) ([]string, string, int, error) {
	positionals := []string{}
	root := ""
	limit := 10
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--root":
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" {
				return nil, "", 0, fmt.Errorf("missing root")
			}
			root = strings.TrimSpace(args[index])
		case "--limit":
			if !allowLimit {
				return nil, "", 0, fmt.Errorf("limit is not supported")
			}
			index++
			if index >= len(args) {
				return nil, "", 0, fmt.Errorf("missing limit")
			}
			parsed, err := strconv.Atoi(args[index])
			if err != nil || parsed < 1 || parsed > 100 {
				return nil, "", 0, fmt.Errorf("invalid limit")
			}
			limit = parsed
		default:
			if strings.HasPrefix(args[index], "--") {
				return nil, "", 0, fmt.Errorf("unknown option")
			}
			positionals = append(positionals, args[index])
		}
	}
	return positionals, root, limit, nil
}

func resolvePersonalMemoryRoot(root string) (string, error) {
	if strings.TrimSpace(root) != "" {
		return root, nil
	}
	controlRoot, err := privateio.DefaultControlPlaneRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(controlRoot, "personal-memory"), nil
}

func (r Runner) readPersonalMemoryInput(path string) ([]byte, error) {
	if path == "-" {
		input, err := io.ReadAll(io.LimitReader(r.nativeInput, maximumNativeBatchBytes+1))
		if err != nil || len(input) > maximumNativeBatchBytes {
			return nil, fmt.Errorf("personal memory input exceeds size limit")
		}
		return input, nil
	}
	return r.fs.ReadFileBounded(path, maximumNativeBatchBytes)
}

func encodePersonalMemoryJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "write personal evidence response: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}
