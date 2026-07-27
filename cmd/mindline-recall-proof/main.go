// mindline-recall-proof validates WP-48 structural receipts without printing
// owner-only manifests, queries, record identities, or canonical evidence.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/recallproof"
)

func main() {
	if err := run(os.Args[1:], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "mindline-recall-proof:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader) error {
	if len(args) == 1 && args[0] == "live-config-fingerprint" {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"schema_version": "mindline-recall-live-configuration-receipt/v0.1",
			"fingerprint":    recallproof.LiveConfigurationFingerprint(),
		})
	}
	if len(args) != 2 {
		return errors.New("usage: live-config-fingerprint | run-pre-live - | authorize-pre-live <owner-dir> | validate-artifact <structural.json> | validate-authority <receipt.json> | validate-phase <phase-receipt.json>")
	}
	if args[0] == "run-pre-live" {
		if args[1] != "-" {
			return errors.New("run-pre-live accepts its binding only on stdin")
		}
		data, err := io.ReadAll(io.LimitReader(stdin, 64<<10))
		if err != nil {
			return errors.New("read pre-live binding")
		}
		var binding recallproof.TreeConfigBinding
		if err := decodeStrict(data, &binding); err != nil {
			return err
		}
		root, err := os.Getwd()
		if err != nil {
			return errors.New("resolve repository root")
		}
		artifact, err := (recallproof.ReusableProofRunner{
			Executor: recallproof.OSDirectExecutor{},
		}).RunPreLive(context.Background(), root, binding)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(artifact)
	}
	if args[0] == "authorize-pre-live" {
		data, err := io.ReadAll(io.LimitReader(stdin, 64<<10))
		if err != nil {
			return errors.New("read pre-live binding")
		}
		var binding recallproof.TreeConfigBinding
		if err := decodeStrict(data, &binding); err != nil {
			return err
		}
		repositoryRoot, err := os.Getwd()
		if err != nil {
			return errors.New("resolve repository root")
		}
		artifact, err := (recallproof.ReusableProofRunner{
			Executor: recallproof.OSDirectExecutor{},
		}).RunPreLive(context.Background(), repositoryRoot, binding)
		if err != nil {
			return err
		}
		receipt, err := recallproof.AuthorityReceiptFromPreLive(recallproof.PhaseReceipt{
			SchemaVersion: recallproof.PhaseReceiptSchema, Phase: "pre_live",
			Binding: binding, Artifact: artifact,
		})
		if err != nil {
			return err
		}
		root := filepath.Clean(args[1])
		if !filepath.IsAbs(root) {
			return errors.New("authority receipt root must be absolute")
		}
		if err := privateio.PrepareDir(root); err != nil {
			return errors.New("prepare owner-only authority receipt root")
		}
		if err := privateio.WriteJSONNoReplace(filepath.Join(root, "wp48-pre-live-authority.json"), receipt); err != nil {
			return errors.New("persist owner-only authority receipt")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"schema_version": "mindline-recall-authority-mint/v0.1",
			"state":          "pass",
			"binding":        receipt.Binding,
			"fingerprint":    receipt.ReusableProofFingerprint,
		})
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	switch args[0] {
	case "validate-artifact":
		artifact, err := recallproof.DecodeStructuralArtifact(data)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(artifact)
	case "validate-authority":
		var receipt recallproof.AuthorityReceipt
		if err := decodeStrict(data, &receipt); err != nil {
			return err
		}
		if err := receipt.Validate(); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": "mindline-recall-proof-command/v0.1", "state": "pass", "binding": receipt.Binding})
	case "validate-phase":
		var receipt recallproof.PhaseReceipt
		if err := decodeStrict(data, &receipt); err != nil {
			return err
		}
		artifact, err := receipt.DecisionArtifact()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(artifact)
	default:
		return errors.New("unknown command")
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("receipt contains trailing JSON")
	}
	return nil
}
