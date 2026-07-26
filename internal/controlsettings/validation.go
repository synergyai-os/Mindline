package controlsettings

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

var (
	generationPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	adapterKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	schemaPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,255}$`)
	stableIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
	secretValuePattern = regexp.MustCompile(`(?i)(xox[baprs]-|pb_sk_|sk-[a-z0-9_-]{16,}|bearer[[:space:]]+[a-z0-9._-]{12,}|api[_ -]?key[[:space:]]*[:=])`)
	secretFieldPattern = regexp.MustCompile(`(?i)^(authorization|cookie|credential|credentials|password|secret|token|access_token|refresh_token|api_key|session|csrf|nonce)$`)
	urlOrPathPattern   = regexp.MustCompile(`(?i)(https?://|file://|(^|[[:space:]])/(Users|home|tmp|private|var)/)`)
)

func randomGeneration(reader io.Reader) (string, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", errors.New("control settings generation unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func canonicalizeDraft(draft Draft, validators map[string]AdapterValidator) (Draft, error) {
	copyDraft := draft
	copyDraft.ContextLenses = append([]string(nil), draft.ContextLenses...)
	copyDraft.AdapterDefaults = make([]AdapterDefault, len(draft.AdapterDefaults))
	if len(copyDraft.ContextLenses) == 0 {
		return Draft{}, ErrInvalid
	}
	for _, lens := range copyDraft.ContextLenses {
		if !validText(lens, 1, 8192) || containsSecret(lens) || urlOrPathPattern.MatchString(lens) {
			return Draft{}, ErrInvalid
		}
	}
	if !validText(copyDraft.RoutingPolicy, 1, 32768) || containsSecret(copyDraft.RoutingPolicy) || urlOrPathPattern.MatchString(copyDraft.RoutingPolicy) {
		return Draft{}, ErrInvalid
	}
	policy := copyDraft.DrainPolicy
	if policy.MaximumNetworkRequests < 1 || policy.MaximumNetworkRequests > 1000000 ||
		policy.MaximumWallTimeSeconds < 60 || policy.MaximumWallTimeSeconds > 86400 ||
		policy.MaximumCostMicrounits < 0 || policy.MaximumCostMicrounits > 1000000000000 ||
		policy.MaximumRetryAttempts < 0 || policy.MaximumRetryAttempts > 100000 ||
		policy.ManualSupportTolerance < 0 || policy.ManualSupportTolerance > 250000 {
		return Draft{}, ErrInvalid
	}
	if len(draft.AdapterDefaults) > 16 {
		return Draft{}, ErrInvalid
	}
	seenSlots := make(map[string]struct{}, len(draft.AdapterDefaults))
	for index, envelope := range draft.AdapterDefaults {
		if envelope.Slot != "source" && envelope.Slot != "destination" {
			return Draft{}, ErrInvalid
		}
		if _, duplicate := seenSlots[envelope.Slot]; duplicate {
			return Draft{}, ErrInvalid
		}
		seenSlots[envelope.Slot] = struct{}{}
		if !adapterKindPattern.MatchString(envelope.AdapterKind) || !schemaPattern.MatchString(envelope.SchemaVersion) || strings.Contains(envelope.SchemaVersion, "..") {
			return Draft{}, ErrInvalid
		}
		validator := validators[envelope.AdapterKind]
		if validator == nil || len(envelope.Values) == 0 || len(envelope.Values) > 16*1024 {
			return Draft{}, ErrInvalid
		}
		if err := rejectSecretJSON(envelope.Values); err != nil {
			return Draft{}, ErrInvalid
		}
		canonical, err := validator.ValidateDefaults(envelope.SchemaVersion, append(json.RawMessage(nil), envelope.Values...))
		if err != nil || len(canonical) == 0 || len(canonical) > 16*1024 || rejectSecretJSON(canonical) != nil {
			return Draft{}, ErrInvalid
		}
		var value any
		if err := privateio.DecodeJSONStrict(canonical, &value); err != nil {
			return Draft{}, ErrInvalid
		}
		canonical, err = json.Marshal(value)
		if err != nil || len(canonical) == 0 || canonical[0] != '{' {
			return Draft{}, ErrInvalid
		}
		copyDraft.AdapterDefaults[index] = AdapterDefault{
			Slot: envelope.Slot, AdapterKind: envelope.AdapterKind,
			SchemaVersion: envelope.SchemaVersion, Values: canonical,
		}
	}
	if err := validateIdentity(copyDraft.ExpectedSourceIdentity); err != nil {
		return Draft{}, ErrInvalid
	}
	if err := validateIdentity(copyDraft.ExpectedDestinationIdentity); err != nil {
		return Draft{}, ErrInvalid
	}
	return copyDraft, nil
}

func validateIdentity(identity *ExpectedIdentity) error {
	if identity == nil {
		return nil
	}
	if !adapterKindPattern.MatchString(identity.AdapterKind) || !stableIDPattern.MatchString(identity.WorkspaceID) || !schemaPattern.MatchString(identity.CapabilityVersion) || strings.Contains(identity.CapabilityVersion, "..") {
		return ErrInvalid
	}
	values := []string{identity.AdapterKind, identity.WorkspaceID, identity.CapabilityVersion}
	if identity.ChannelID != "" {
		if !stableIDPattern.MatchString(identity.ChannelID) {
			return ErrInvalid
		}
		values = append(values, identity.ChannelID)
	}
	if identity.KeyID != "" {
		if !stableIDPattern.MatchString(identity.KeyID) {
			return ErrInvalid
		}
		values = append(values, identity.KeyID)
	}
	for _, value := range values {
		if containsSecret(value) || urlOrPathPattern.MatchString(value) {
			return ErrInvalid
		}
	}
	return nil
}

func validText(value string, minimum, maximum int) bool {
	return utf8.ValidString(value) && len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value
}

func containsSecret(value string) bool {
	return secretValuePattern.MatchString(value)
}

func rejectSecretJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrInvalid
	}
	if err := privateio.DecodeJSONStrict(raw, new(any)); err != nil {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return ErrInvalid
	}
	var inspect func(any) bool
	inspect = func(node any) bool {
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if secretFieldPattern.MatchString(key) || inspect(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if inspect(child) {
					return true
				}
			}
		case string:
			return containsSecret(typed)
		}
		return false
	}
	if inspect(value) {
		return ErrInvalid
	}
	return nil
}

func sealDocument(version uint64, generation string, savedAt time.Time, draft Draft) (Document, error) {
	document := Document{
		SchemaVersion: SchemaVersion, Version: version, Generation: generation,
		SavedAt: savedAt.UTC().Truncate(time.Second).Format(time.RFC3339), Draft: draft,
	}
	document.Fingerprint = fingerprintDocument(document)
	return document, nil
}

func validateDocument(document Document, validators map[string]AdapterValidator) (Document, error) {
	if document.SchemaVersion != SchemaVersion {
		return Document{}, ErrUnsupported
	}
	if document.Version == 0 || !generationPattern.MatchString(document.Generation) {
		return Document{}, ErrInvalid
	}
	parsedTime, err := time.Parse(time.RFC3339, document.SavedAt)
	if err != nil || parsedTime.Format(time.RFC3339) != document.SavedAt {
		return Document{}, ErrInvalid
	}
	draft, err := canonicalizeDraft(document.Draft, validators)
	if err != nil {
		return Document{}, ErrInvalid
	}
	document.Draft = draft
	if document.Fingerprint != fingerprintDocument(document) {
		return Document{}, ErrInvalid
	}
	return document, nil
}

func fingerprintDocument(document Document) string {
	projection := struct {
		SchemaVersion string `json:"schema_version"`
		Version       uint64 `json:"version"`
		Generation    string `json:"generation"`
		SavedAt       string `json:"saved_at"`
		Draft         Draft  `json:"draft"`
	}{document.SchemaVersion, document.Version, document.Generation, document.SavedAt, document.Draft}
	data, _ := json.Marshal(projection)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseDocument(raw []byte, validators map[string]AdapterValidator) (Document, error) {
	if int64(len(raw)) > MaxDocumentBytes {
		return Document{}, ErrInvalid
	}
	var document Document
	if err := privateio.DecodeJSONStrict(raw, &document); err != nil {
		return Document{}, ErrInvalid
	}
	return validateDocument(document, validators)
}

func canonicalDocumentBytes(document Document) ([]byte, error) {
	data, err := json.Marshal(document)
	if err != nil || int64(len(data)+1) > MaxDocumentBytes {
		return nil, ErrInvalid
	}
	return append(data, '\n'), nil
}

func problemFingerprint(code string, current, backup []byte) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00", code)
	currentSum := sha256.Sum256(current)
	backupSum := sha256.Sum256(backup)
	_, _ = hash.Write(currentSum[:])
	_, _ = hash.Write(backupSum[:])
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
