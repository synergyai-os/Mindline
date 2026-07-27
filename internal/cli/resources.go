package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcefetch"
	"github.com/synergyai-os/Mindline/internal/resourcepipeline"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
)

// runResourceCommand exposes only structural state. In particular, neither
// errors nor JSON responses contain a canonical root, queue root, URL, or
// fetched material.
func (r Runner) runResourceCommand(args []string, stdout, stderr io.Writer, command string) int {
	retryReason := ""
	if command == "retry" {
		filtered := make([]string, 0, len(args))
		for index := 0; index < len(args); index++ {
			if args[index] != "--reason" {
				filtered = append(filtered, args[index])
				continue
			}
			index++
			if index >= len(args) || retryReason != "" {
				fmt.Fprint(stderr, usage)
				return ExitUsage
			}
			retryReason = args[index]
		}
		args = filtered
		if !resourcequeue.IsRetryableTerminalReason(retryReason) {
			fmt.Fprint(stderr, usage)
			return ExitUsage
		}
	}
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open resource processing: unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintln(stderr, "open resource processing: unavailable")
		return ExitProcess
	}
	// The queue is derived state beside, never within, the canonical root.
	pipeline, err := resourcepipeline.NewLive(filepath.Join(filepath.Dir(root), "resource-queue"), repository, resourcequeue.LiveProfile(), resourcefetchDependencies())
	if err != nil {
		fmt.Fprintln(stderr, "open resource processing: unavailable")
		return ExitProcess
	}
	if command == "reconcile" {
		if err := pipeline.Reconcile(context.Background()); err != nil {
			fmt.Fprintln(stderr, "reconcile resource processing: unavailable")
			return ExitProcess
		}
		status, err := pipeline.StructuralStatus()
		if err != nil {
			fmt.Fprintln(stderr, "read reconciled resource processing: unavailable")
			return ExitProcess
		}
		return encodePersonalMemoryJSON(stdout, stderr, status)
	}
	if command == "run" || command == "continue" || command == "retry" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		var runErr error
		if command == "retry" {
			runErr = pipeline.Retry(ctx, retryReason)
		} else if command == "continue" {
			runErr = pipeline.Continue(ctx)
		} else {
			runErr = pipeline.Run(ctx)
		}
		status, statusErr := pipeline.StructuralStatus()
		if statusErr != nil {
			fmt.Fprintln(stderr, "resource processing did not reach a terminal state")
			return ExitProcess
		}
		queue, queueErr := pipeline.Store.Load()
		if queueErr != nil || !resourceQueueTerminal(queue) {
			fmt.Fprintln(stderr, "resource processing did not reach a terminal state")
			return ExitProcess
		}
		if code := encodePersonalMemoryJSON(stdout, stderr, status); code != ExitOK {
			return code
		}
		if runErr != nil {
			fmt.Fprintln(stderr, "resource processing did not reach a terminal state")
			return ExitProcess
		}
		return ExitOK
	}
	if command == "status" {
		status, err := pipeline.StructuralStatus()
		if err != nil {
			fmt.Fprintln(stderr, "read resource processing status: unavailable")
			return ExitProcess
		}
		return encodePersonalMemoryJSON(stdout, stderr, status)
	}
	if command == "rebuild" {
		before, err := personalMemoryCanonicalReadback(repository)
		if err != nil {
			fmt.Fprintln(stderr, "read resource rebuild proof: unavailable")
			return ExitProcess
		}
		after, err := pipeline.DeleteAndRebuild(func() (resourcepipeline.CanonicalReadback, error) {
			return personalMemoryCanonicalReadback(repository)
		})
		if err != nil {
			fmt.Fprintln(stderr, "rebuild resource queue: unavailable")
			return ExitProcess
		}
		queue, err := pipeline.Store.Load()
		if err != nil || !resourceQueueTerminal(queue) {
			fmt.Fprintln(stderr, "rebuilt resource queue is not terminal")
			return ExitProcess
		}
		proof := map[string]any{
			"schema_version":     "mindline-resource-queue-rebuild-proof/v0.1",
			"state":              "pass",
			"canonical_before":   before.Canonical,
			"canonical_after":    after.Canonical,
			"compact_before":     before.Compact,
			"compact_after":      after.Compact,
			"get_before":         before.Get,
			"get_after":          after.Get,
			"all_terminal":       true,
			"terminal_resources": len(queue.Items),
		}
		return encodePersonalMemoryJSON(stdout, stderr, proof)
	}
	proof, err := pipeline.StructuralProof()
	if err != nil {
		fmt.Fprintln(stderr, "read resource processing proof: unavailable")
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, proof)
}

func resourceQueueTerminal(queue resourcequeue.Queue) bool {
	for _, item := range queue.Items {
		if item.State != resourcequeue.StateComplete && item.State != resourcequeue.StatePartial && item.State != resourcequeue.StateBlocked {
			return false
		}
	}
	return true
}

func personalMemoryCanonicalReadback(repository *personalmemory.FileRepository) (resourcepipeline.CanonicalReadback, error) {
	library, err := repository.Load()
	if err != nil {
		return resourcepipeline.CanonicalReadback{}, err
	}
	canonical := "sha256:" + library.Fingerprint
	retriever := personalmemory.NewLexicalRetriever(repository)
	compact, err := retriever.SearchCompact(personalmemory.SearchRequest{
		Query: "product knowledge lessons ideas", Limit: 8,
	})
	if err != nil {
		return resourcepipeline.CanonicalReadback{}, err
	}
	compactBytes, err := json.Marshal(compact)
	if err != nil {
		return resourcepipeline.CanonicalReadback{}, err
	}
	ids := make([]string, 0, len(library.Records))
	for _, record := range library.Records {
		ids = append(ids, record.RecordID)
	}
	sort.Strings(ids)
	if len(ids) > 16 {
		ids = ids[:16]
	}
	getHash := sha256.New()
	for _, id := range ids {
		hydrated, err := retriever.Get(id)
		if err != nil {
			return resourcepipeline.CanonicalReadback{}, err
		}
		payload, err := json.Marshal(hydrated)
		if err != nil {
			return resourcepipeline.CanonicalReadback{}, err
		}
		getHash.Write(payload)
		getHash.Write([]byte{0})
	}
	compactSum := sha256.Sum256(compactBytes)
	return resourcepipeline.CanonicalReadback{
		Canonical: canonical,
		Compact:   "sha256:" + hex.EncodeToString(compactSum[:]),
		Get:       "sha256:" + hex.EncodeToString(getHash.Sum(nil)),
	}, nil
}

// Kept as a small factory so the CLI always selects the resourcefetch defaults
// and tests can remain entirely network-free by exercising empty queues.
func resourcefetchDependencies() resourcefetch.Dependencies { return resourcefetch.Dependencies{} }
