package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func (r Runner) runProductBrainOutbox(args []string, stdout, stderr io.Writer) int {
	routingDir, profilePath, outDir, ok := parseThreePathCommand(args, "outbox", "--profile")
	if !ok {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if err := validatePrivateRuntimePaths(routingDir, profilePath, outDir); err != nil {
		fmt.Fprintf(stderr, "invalid private runtime path: %v\n", err)
		return ExitUsage
	}
	if code := r.prepareProductBrainOutput(outDir, stderr); code != ExitOK {
		return code
	}
	route, err := routing.LoadResult(routingDir)
	if err != nil {
		fmt.Fprintf(stderr, "load routing artifacts: %v\n", err)
		return ExitProcess
	}
	profile, err := productbrain.LoadDeliveryProfile(profilePath)
	if err != nil {
		fmt.Fprintf(stderr, "load Product Brain delivery profile: %v\n", err)
		return ExitProcess
	}
	outbox, summary, err := productbrain.CompileOutbox(route, profile)
	if err != nil {
		fmt.Fprintf(stderr, "compile Product Brain outbox: %v\n", err)
		return ExitProcess
	}
	if err := productbrain.WriteOutbox(outDir, outbox, summary); err != nil {
		fmt.Fprintf(stderr, "write Product Brain outbox: %v\n", err)
		return ExitArtifactWrite
	}
	return encodeJSON(stdout, stderr, summary)
}

func (r Runner) runProductBrainPreflight(args []string, stdout, stderr io.Writer) int {
	outboxDir, outDir, ok := parseTwoPathCommand(args, "preflight", "--out")
	if !ok {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if err := validatePrivateRuntimePaths(outboxDir, outDir); err != nil {
		fmt.Fprintf(stderr, "invalid private runtime path: %v\n", err)
		return ExitUsage
	}
	if code := r.prepareProductBrainOutput(outDir, stderr); code != ExitOK {
		return code
	}
	outbox, err := productbrain.LoadOutbox(outboxDir)
	if err != nil {
		fmt.Fprintf(stderr, "load Product Brain outbox: %v\n", err)
		return ExitProcess
	}
	profile := profileFromSnapshot(outbox.ProfileSnapshot)
	provider := r.productBrainSecretProvider
	if provider == nil {
		provider = productbrain.EnvironmentSecretProvider{Name: "MINDLINE_PRODUCT_BRAIN_API_KEY"}
	}
	transport, err := r.newProductBrainRuntimeTransport(context.Background(), profile, provider)
	if err != nil {
		fmt.Fprintf(stderr, "construct Product Brain transport: %v\n", err)
		return ExitProcess
	}
	artifact, err := productbrain.BuildPreflight(context.Background(), outbox, profile, transport)
	if err != nil {
		fmt.Fprintf(stderr, "Product Brain preflight: %v\n", err)
		return ExitProcess
	}
	if err := productbrain.WritePreflight(outDir, artifact); err != nil {
		fmt.Fprintf(stderr, "write Product Brain preflight: %v\n", err)
		return ExitArtifactWrite
	}
	return encodeJSON(stdout, stderr, artifact)
}

func (r Runner) runProductBrainDeliver(args []string, stdout, stderr io.Writer) int {
	outboxDir, preflightDir, outDir, ok := parseThreePathCommand(args, "deliver", "--preflight")
	if !ok {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if err := validatePrivateRuntimePaths(outboxDir, preflightDir, outDir); err != nil {
		fmt.Fprintf(stderr, "invalid private runtime path: %v\n", err)
		return ExitUsage
	}
	if code := r.prepareProductBrainOutput(outDir, stderr); code != ExitOK {
		return code
	}
	outbox, err := productbrain.LoadOutbox(outboxDir)
	if err != nil {
		fmt.Fprintf(stderr, "load Product Brain outbox: %v\n", err)
		return ExitProcess
	}
	profile := profileFromSnapshot(outbox.ProfileSnapshot)
	preflight, err := productbrain.LoadPreflight(filepath.Join(preflightDir, "preflight.json"))
	if err != nil {
		fmt.Fprintf(stderr, "load Product Brain preflight: %v\n", err)
		return ExitProcess
	}
	provider := r.productBrainSecretProvider
	if provider == nil {
		provider = productbrain.EnvironmentSecretProvider{Name: "MINDLINE_PRODUCT_BRAIN_API_KEY"}
	}
	transport, err := r.newProductBrainRuntimeTransport(context.Background(), profile, provider)
	if err != nil {
		fmt.Fprintf(stderr, "construct Product Brain transport: %v\n", err)
		return ExitProcess
	}
	summary, err := productbrain.Deliver(context.Background(), outbox, profile, preflight, transport, outDir, productbrain.DeliveryOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "deliver Product Brain outbox: %v\n", err)
		return ExitProcess
	}
	return encodeJSON(stdout, stderr, summary)
}

func (r Runner) runProductBrainReview(args []string, stdout, stderr io.Writer) int {
	if len(args) != 8 || args[0] != "review" || args[2] != "--outbox" || args[4] != "--delivery" || args[6] != "--out" {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	routingDir, outboxDir, deliveryDir, outDir := args[1], args[3], args[5], args[7]
	if err := validatePrivateRuntimePaths(routingDir, outboxDir, deliveryDir, outDir); err != nil {
		fmt.Fprintf(stderr, "invalid private runtime path: %v\n", err)
		return ExitUsage
	}
	if code := r.prepareProductBrainOutput(outDir, stderr); code != ExitOK {
		return code
	}
	route, err := routing.LoadResult(routingDir)
	if err != nil {
		fmt.Fprintf(stderr, "load routing artifacts: %v\n", err)
		return ExitProcess
	}
	outbox, err := productbrain.LoadOutbox(outboxDir)
	if err != nil {
		fmt.Fprintf(stderr, "load Product Brain outbox: %v\n", err)
		return ExitProcess
	}
	profile := profileFromSnapshot(outbox.ProfileSnapshot)
	summary, err := productbrain.WriteIntegratedReview(outDir, deliveryDir, route, outbox, profile)
	if err != nil {
		fmt.Fprintf(stderr, "write integrated Product Brain review: %v\n", err)
		return ExitArtifactWrite
	}
	return encodeJSON(stdout, stderr, summary)
}

func (r Runner) prepareProductBrainOutput(outDir string, stderr io.Writer) int {
	if err := r.validateDestinationOutDir(outDir); err != nil {
		fmt.Fprintf(stderr, "invalid Product Brain output: %v\n", err)
		return ExitUsage
	}
	if err := privateio.PrepareDir(outDir); err != nil {
		fmt.Fprintf(stderr, "prepare Product Brain output: %v\n", err)
		return ExitArtifactWrite
	}
	return ExitOK
}

func parseThreePathCommand(args []string, command, middleFlag string) (input, middle, out string, ok bool) {
	if len(args) != 6 || args[0] != command || args[2] != middleFlag || args[4] != "--out" {
		return
	}
	input, middle, out = args[1], args[3], args[5]
	return input, middle, out, input != "" && middle != "" && out != ""
}
func parseTwoPathCommand(args []string, command, flag string) (input, out string, ok bool) {
	if len(args) != 4 || args[0] != command || args[2] != flag {
		return
	}
	return args[1], args[3], args[1] != "" && args[3] != ""
}
func profileFromSnapshot(s productbrain.DeliveryProfileSnapshot) productbrain.DeliveryProfile {
	return productbrain.DeliveryProfileFromSnapshot(s)
}

func (r Runner) newProductBrainRuntimeTransport(ctx context.Context, profile productbrain.DeliveryProfile, provider productbrain.SecretProvider) (productbrain.ProductBrainTransport, error) {
	switch profile.Transport.Kind {
	case "aki":
		return productbrain.NewAKITransport(ctx, profile, provider, r.productBrainTransport)
	default:
		return nil, fmt.Errorf("unsupported Product Brain transport: %s", profile.Transport.Kind)
	}
}
