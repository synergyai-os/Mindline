// mindline-recall-eval runs owner-only same-library retrieval evaluation while
// emitting only structural receipts to stdout.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/recalleval"
)

const maximumOwnerEvalBytes = 8 << 20

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mindline-recall-eval: operation failed")
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 3 && args[0] == "seal" {
		data, err := io.ReadAll(io.LimitReader(stdin, maximumOwnerEvalBytes+1))
		if err != nil || len(data) > maximumOwnerEvalBytes {
			return errors.New("invalid draft input")
		}
		var draft recalleval.DraftManifest
		if err := privateio.DecodeJSONStrict(data, &draft); err != nil {
			return err
		}
		client := localservice.NewClient(args[1])
		port := recalleval.LocalServicePort{Client: client, Mode: recalleval.ServiceModeCompact}
		manifest, err := recalleval.SealOwnerManifest(context.Background(), draft, port)
		if err != nil {
			return err
		}
		if err := privateio.WriteJSON(args[2], manifest); err != nil {
			return err
		}
		return encode(stdout, map[string]any{
			"schema_version": "mindline-retrieval-eval-seal-receipt/v0.1",
			"state":          "sealed", "case_count": len(manifest.Cases),
			"manifest_fingerprint": manifest.Fingerprint,
		})
	}
	if len(args) == 6 && args[0] == "run" {
		mode := args[1]
		if mode != recalleval.ServiceModeLegacy && mode != recalleval.ServiceModeCompact {
			return errors.New("invalid evaluation mode")
		}
		var manifest recalleval.OwnerManifest
		if err := readStrict(args[3], &manifest); err != nil {
			return err
		}
		var binding recalleval.RunBinding
		if err := readStrict(args[4], &binding); err != nil {
			return err
		}
		port := recalleval.LocalServicePort{
			Client: localservice.NewClient(args[2]), Mode: mode,
		}
		result, err := recalleval.Run(context.Background(), manifest, binding, port, port)
		if err != nil {
			return err
		}
		if err := privateio.WriteJSON(args[5], result); err != nil {
			return err
		}
		return encode(stdout, map[string]any{
			"schema_version": "mindline-retrieval-eval-run-receipt/v0.1",
			"state":          "complete", "mode": mode,
			"result_fingerprint": fingerprint(result),
		})
	}
	if len(args) == 4 && args[0] == "compare" {
		var manifest recalleval.OwnerManifest
		var baseline, candidate recalleval.RunResult
		if readStrict(args[1], &manifest) != nil ||
			readStrict(args[2], &baseline) != nil ||
			readStrict(args[3], &candidate) != nil {
			return errors.New("invalid comparison input")
		}
		result, err := recalleval.CompareRuns(manifest, baseline, candidate)
		if err != nil {
			return err
		}
		return encode(stdout, result)
	}
	return errors.New("usage: seal <socket> <owner-manifest> | run legacy|compact <socket> <owner-manifest> <binding> <owner-result> | compare <owner-manifest> <baseline-result> <candidate-result>")
}

func readStrict(path string, target any) error {
	clean := filepath.Clean(path)
	root, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return errors.New("owner evaluation input unavailable")
	}
	clean = filepath.Join(root, filepath.Base(clean))
	if err := privateio.ReadJSONStrictBounded(root, clean, maximumOwnerEvalBytes, target); err != nil {
		return errors.New("owner evaluation input unavailable")
	}
	return nil
}

func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func encode(writer io.Writer, value any) error {
	return json.NewEncoder(writer).Encode(value)
}
