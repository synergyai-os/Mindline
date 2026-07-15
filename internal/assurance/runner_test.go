package assurance

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFixedGateExecutesOnlyPinnedChecks(t *testing.T) {
	var executed []string
	context := gateRunContext{
		sourceRoot: "/workspace", cleanHEADRoot: "/clean-head", cleanHEADBinding: "sha256:clean",
		runtimeSnapshotRoot: "/runtime-snapshot", runtimeSnapshotBinding: "sha256:runtime",
	}
	checks, err := runFixedGate(context, func(name string, args []string, workdir string) ([]byte, error) {
		executed = append(executed, workdir+" "+name+" "+joinArgs(args))
		switch name {
		case "govulncheck":
			return []byte("Scanner: govulncheck@v1.1.4"), nil
		case "gosec":
			return []byte("Version: 2.28.0"), nil
		case "gitleaks":
			return []byte("8.30.1"), nil
		default:
			return []byte(name + " fixed-version"), nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, check := range checks {
		names = append(names, check.Name)
	}
	if !reflect.DeepEqual(names, RequiredChecks) {
		t.Fatalf("fixed gate check order drifted: %v", names)
	}
	if len(executed) != len(fixedGateCommands)*2 {
		t.Fatalf("expected version and check execution for every fixed command, got %d", len(executed))
	}
	assertCommandSurface(t, executed, "gitleaks dir --redact --no-banner .", "/clean-head")
	assertCommandSurface(t, executed, "gitleaks git --redact --no-banner .", "/workspace")
	assertCommandSurface(t, executed, "gitleaks dir --redact --no-banner .", "/runtime-snapshot")
}

func TestFixedGateCannotMintReceiptAfterCommandFailure(t *testing.T) {
	context := gateRunContext{
		sourceRoot: "/workspace", cleanHEADRoot: "/clean-head", cleanHEADBinding: "sha256:clean",
		runtimeSnapshotRoot: "/runtime-snapshot", runtimeSnapshotBinding: "sha256:runtime",
	}
	_, err := runFixedGate(context, func(name string, args []string, _ string) ([]byte, error) {
		if name == "go" && len(args) > 0 && args[0] == "vet" {
			return nil, errors.New("failed")
		}
		switch name {
		case "govulncheck":
			return []byte("govulncheck@v1.1.4"), nil
		case "gosec":
			return []byte("Version: 2.28.0"), nil
		case "gitleaks":
			return []byte("8.30.1"), nil
		default:
			return []byte("fixed-version"), nil
		}
	})
	if err == nil {
		t.Fatal("failed fixed check minted authority")
	}
}

func TestGosecPreLivePolicyIsExplicitlyHighSeverityHighConfidence(t *testing.T) {
	want := []string{"-quiet", "-fmt=json", "-severity=high", "-confidence=high", "-exclude-generated", "./..."}
	for _, command := range fixedGateCommands {
		if command.name == "gosec" {
			if !reflect.DeepEqual(command.args, want) {
				t.Fatalf("gosec authority policy drifted: %v", command.args)
			}
			return
		}
	}
	t.Fatal("gosec authority check is missing")
}

func assertCommandSurface(t *testing.T, executed []string, command, expectedRoot string) {
	t.Helper()
	for _, value := range executed {
		if strings.HasSuffix(value, command) && strings.HasPrefix(value, expectedRoot+" ") {
			return
		}
	}
	t.Fatalf("command %q did not execute on %q: %v", command, expectedRoot, executed)
}

func joinArgs(args []string) string {
	result := ""
	for index, value := range args {
		if index > 0 {
			result += " "
		}
		result += value
	}
	return result
}
