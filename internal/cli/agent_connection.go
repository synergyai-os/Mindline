package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/synergyai-os/Mindline/internal/localservice"
)

func (r Runner) runAgentConnectionHandle(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		return agentUsage(stderr)
	}
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return writeAgentContractError(stderr, "connection_handle", "token_generation_failed", true, "retry")
	}
	return encodePersonalMemoryJSON(stdout, stderr, map[string]any{
		"schema_version": "mindline-project-connection-handle/v0.1",
		"connection":     "mlc1_" + base64.RawURLEncoding.EncodeToString(value),
		"secret":         false,
		"owner":          "caller",
	})
}

func (r Runner) runAgentConnectionBind(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 ||
		!onlyAgentKeys(options.values, "connection", "scope", "lens", "agent") ||
		options.values["connection"] == "" || options.values["scope"] == "" ||
		options.values["lens"] == "" || options.values["agent"] == "" {
		return agentUsage(stderr)
	}
	digest, err := projectConnectionDigest(options.values["connection"])
	if err != nil {
		return writeAgentContractError(stderr, "connection_bind", "invalid_connection", false, "create_connection_handle")
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return writeAgentContractError(stderr, "connection_bind", "service_unavailable", true, "retry_service")
	}
	receipt, err := client.BindProjectConnection(context.Background(), localservice.ProjectConnectionInput{
		Digest: digest, ScopeID: options.values["scope"], LensID: options.values["lens"], AgentID: options.values["agent"],
	})
	if err != nil {
		return writeAgentContractError(stderr, "connection_bind", "connection_rejected", false, "inspect_owner_binding")
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runAgentConnectionArchive(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 ||
		!onlyAgentKeys(options.values, "connection") || options.values["connection"] == "" {
		return agentUsage(stderr)
	}
	digest, err := projectConnectionDigest(options.values["connection"])
	if err != nil {
		return writeAgentContractError(stderr, "connection_archive", "invalid_connection", false, "inspect_owner_binding")
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return writeAgentContractError(stderr, "connection_archive", "service_unavailable", true, "retry_service")
	}
	receipt, err := client.ArchiveProjectConnection(context.Background(), digest)
	if err != nil {
		return writeAgentContractError(stderr, "connection_archive", "connection_unavailable", false, "inspect_owner_binding")
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func projectConnectionDigest(handle string) (string, error) {
	if handle != strings.TrimSpace(handle) || !strings.HasPrefix(handle, "mlc1_") || len(handle) != 48 {
		return "", errors.New("invalid project connection handle")
	}
	encoded := strings.TrimPrefix(handle, "mlc1_")
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return "", errors.New("invalid project connection handle")
	}
	digest := sha256.Sum256([]byte("mindline-project-connection-v1\x00" + handle))
	return hex.EncodeToString(digest[:]), nil
}
