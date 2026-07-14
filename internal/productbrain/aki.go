package productbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const maxAKIResponseBytes = 1 << 20

type AKITransport struct {
	endpoint string
	secret   string
	client   *http.Client
}

func NewAKITransport(ctx context.Context, profile DeliveryProfile, provider SecretProvider, roundTripper http.RoundTripper) (*AKITransport, error) {
	return NewAKITransportWithTrust(ctx, profile, provider, roundTripper, ProductionGatewayOrigin)
}
func NewAKITransportWithTrust(ctx context.Context, profile DeliveryProfile, provider SecretProvider, roundTripper http.RoundTripper, trustedOrigin string) (*AKITransport, error) {
	if err := validateTrustedOrigin(profile.Transport.BaseURL, trustedOrigin); err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, errors.New("credential_missing")
	}
	secret, err := provider.Secret(ctx)
	if err != nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("credential_missing")
	}
	if roundTripper == nil {
		roundTripper = http.DefaultTransport
	}
	client := &http.Client{Transport: roundTripper, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &AKITransport{endpoint: profile.Transport.BaseURL + profile.Transport.APIPath, secret: secret, client: client}, nil
}

func validateTrustedOrigin(raw, trusted string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("untrusted_product_brain_origin")
	}
	if u.String() != trusted || u.Scheme != "https" || u.Host != strings.TrimPrefix(trusted, "https://") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("untrusted_product_brain_origin")
	}
	return nil
}
func environmentSecret(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", errors.New("credential_missing")
	}
	return value, nil
}

type akiEnvelope struct {
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
}

func (t *AKITransport) call(ctx context.Context, fn string, args any, target any, mutation bool) error {
	body, err := json.Marshal(map[string]any{"fn": fn, "args": args})
	if err != nil {
		return &TransportError{Category: "validation_failed"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return &TransportError{Category: "validation_failed"}
	}
	request.Header.Set("Authorization", "Bearer "+t.secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-pb-source", "mindline")
	response, err := t.client.Do(request)
	if err != nil {
		return &TransportError{Category: "transient", MayHaveCommitted: mutation}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAKIResponseBytes+1))
	if err != nil || len(data) > maxAKIResponseBytes {
		return &TransportError{Category: "remote_failure", MayHaveCommitted: mutation}
	}
	var envelope akiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return &TransportError{Category: "remote_failure", MayHaveCommitted: mutation}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.OK {
		return &TransportError{Category: safeHTTPError(response.StatusCode, envelope.Code), MayHaveCommitted: mutation && response.StatusCode >= 500}
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return &TransportError{Category: "remote_failure", MayHaveCommitted: mutation}
	}
	return nil
}
func safeHTTPError(status int, code string) string {
	switch status {
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "already_exists"
	case 400, 422:
		return "validation_failed"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusRequestTimeout:
		return "transient"
	}
	lower := strings.ToLower(code)
	if strings.Contains(lower, "duplicate") || strings.Contains(lower, "already") {
		return "already_exists"
	}
	if status >= 500 {
		return "remote_failure"
	}
	return "remote_failure"
}

func (t *AKITransport) ResolveWorkspace(ctx context.Context) (WorkspaceCapability, error) {
	var raw struct {
		ID             string `json:"_id"`
		Slug           string `json:"slug"`
		GovernanceMode string `json:"governanceMode"`
		KeyScope       string `json:"keyScope"`
		KeyID          string `json:"keyId"`
	}
	if err := t.call(ctx, "resolveWorkspace", map[string]any{}, &raw, false); err != nil {
		return WorkspaceCapability{}, err
	}
	return WorkspaceCapability{ID: raw.ID, Slug: raw.Slug, GovernanceMode: raw.GovernanceMode, KeyScope: raw.KeyScope, KeyID: raw.KeyID}, nil
}
func (t *AKITransport) GetCollectionFields(ctx context.Context, slug string) (CollectionCapability, error) {
	var raw *struct {
		Slug   string `json:"slug"`
		Fields []struct {
			Key      string   `json:"key"`
			Type     string   `json:"type"`
			Required bool     `json:"required"`
			Options  []string `json:"options"`
		} `json:"fields"`
	}
	if err := t.call(ctx, "chain.getCollectionFields", map[string]any{"slug": slug}, &raw, false); err != nil {
		return CollectionCapability{}, err
	}
	if raw == nil {
		return CollectionCapability{Found: false}, nil
	}
	capability := CollectionCapability{Found: true, Slug: raw.Slug}
	for _, field := range raw.Fields {
		options := append([]string{}, field.Options...)
		sort.Strings(options)
		capability.Fields = append(capability.Fields, CollectionFieldCapability{Key: field.Key, Type: field.Type, Required: field.Required, Options: options})
	}
	sort.Slice(capability.Fields, func(i, j int) bool { return capability.Fields[i].Key < capability.Fields[j].Key })
	return capability, nil
}
func (t *AKITransport) GetEntry(ctx context.Context, id string) (EntryReadback, error) {
	var raw *struct {
		DocID          string         `json:"_id"`
		EntryID        string         `json:"entryId"`
		CollectionSlug string         `json:"collectionSlug"`
		Name           string         `json:"name"`
		Status         string         `json:"status"`
		Data           map[string]any `json:"data"`
		SourceRef      string         `json:"sourceRef"`
		SourceExcerpt  string         `json:"sourceExcerpt"`
		CreatedBy      string         `json:"createdBy"`
	}
	if err := t.call(ctx, "chain.getEntry", map[string]any{"entryId": id}, &raw, false); err != nil {
		return EntryReadback{}, err
	}
	if raw == nil {
		return EntryReadback{Found: false}, nil
	}
	return EntryReadback{Found: true, DocID: raw.DocID, EntryID: raw.EntryID, CollectionSlug: raw.CollectionSlug, Name: raw.Name, Status: raw.Status, Data: raw.Data, SourceRef: raw.SourceRef, SourceExcerpt: raw.SourceExcerpt, CreatedBy: raw.CreatedBy}, nil
}
func (t *AKITransport) SearchEntries(ctx context.Context, query, collection string) ([]EntrySearchResult, error) {
	var raw []struct {
		DocID          string `json:"_id"`
		EntryID        string `json:"entryId"`
		CollectionSlug string `json:"collectionSlug"`
		Name           string `json:"name"`
		Status         string `json:"status"`
	}
	if err := t.call(ctx, "chain.searchEntries", map[string]any{"query": query, "collectionSlug": collection}, &raw, false); err != nil {
		return nil, err
	}
	out := []EntrySearchResult{}
	for _, v := range raw {
		if v.Name == query && v.CollectionSlug == collection {
			out = append(out, EntrySearchResult{DocID: v.DocID, EntryID: v.EntryID, CollectionSlug: v.CollectionSlug, Name: v.Name, Status: v.Status})
		}
	}
	return out, nil
}
func (t *AKITransport) CreateEntry(ctx context.Context, r CreateEntryRequest) (CreateEntryResult, error) {
	args := map[string]any{"collectionSlug": r.CollectionSlug, "entryId": r.EntryID, "name": r.Name, "data": r.Data, "forceDraft": r.ForceDraft, "createdBy": r.CreatedBy, "sourceRef": r.SourceRef, "sourceExcerpt": r.SourceExcerpt}
	var raw struct {
		EntryID string `json:"entryId"`
		Status  string `json:"status"`
	}
	if err := t.call(ctx, "chain.createEntry", args, &raw, true); err != nil {
		return CreateEntryResult{}, err
	}
	return CreateEntryResult{EntryID: raw.EntryID, Status: raw.Status}, nil
}
func (t *AKITransport) ListEntryRelations(ctx context.Context, id string) ([]RelationReadback, error) {
	var raw []struct {
		ID       string         `json:"_id"`
		FromID   string         `json:"fromId"`
		ToID     string         `json:"toId"`
		Type     string         `json:"type"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := t.call(ctx, "chain.listEntryRelations", map[string]any{"entryId": id}, &raw, false); err != nil {
		return nil, err
	}
	out := make([]RelationReadback, 0, len(raw))
	for _, v := range raw {
		out = append(out, RelationReadback{RelationID: v.ID, FromDocID: v.FromID, ToDocID: v.ToID, Type: v.Type, Metadata: v.Metadata})
	}
	return out, nil
}
func (t *AKITransport) CreateEntryRelation(ctx context.Context, r CreateRelationRequest) (CreateRelationResult, error) {
	args := map[string]any{"fromEntryId": r.FromEntryID, "toEntryId": r.ToEntryID, "type": r.Type, "metadata": r.Metadata, "ifMissing": r.IfMissing}
	var raw struct {
		RelationID    string `json:"relationId"`
		AlreadyExists bool   `json:"alreadyExists"`
	}
	if err := t.call(ctx, "chain.createEntryRelation", args, &raw, true); err != nil {
		return CreateRelationResult{}, err
	}
	return CreateRelationResult{RelationID: raw.RelationID, AlreadyExists: raw.AlreadyExists}, nil
}

func (t *AKITransport) RuntimeSecretFindings(value any) []PrivacyFinding {
	return ScanPublicArtifact(value, t.secret)
}

var _ ProductBrainTransport = (*AKITransport)(nil)
var _ RuntimeSecretScanner = (*AKITransport)(nil)
var _ = fmt.Sprintf
