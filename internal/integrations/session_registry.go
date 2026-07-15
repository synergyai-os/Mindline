package integrations

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
)

const (
	defaultMaximumSecretBytes = 16 << 10
	handleBytes               = 32
)

var (
	ErrLeaseNotFound       = errors.New("session lease not found")
	ErrLeaseRevoked        = errors.New("session lease revoked")
	ErrRegistryClosed      = errors.New("session registry closed")
	ErrIdentityMismatch    = errors.New("verified connection identity mismatch")
	ErrIdentityNotPinned   = errors.New("verified connection identity not pinned")
	ErrCredentialRejected  = errors.New("provider rejected credential")
	ErrInvalidLeaseOptions = errors.New("invalid session lease options")
)

// SessionRef is an opaque, authorizing reference. It must remain process-local
// and must never be persisted as connection metadata.
type SessionRef string

type ConnectionKind string

const (
	ConnectionSlackWebAPI  ConnectionKind = "slack_web_api"
	ConnectionProductBrain ConnectionKind = "product_brain"
)

// VerifiedIdentity contains only identity returned by the provider. It is safe
// to persist, but it is not authorization and contains no usable credential.
type VerifiedIdentity struct {
	Provider          string `json:"provider"`
	WorkspaceID       string `json:"workspace_id"`
	ChannelID         string `json:"channel_id,omitempty"`
	KeyID             string `json:"key_id,omitempty"`
	CapabilityVersion string `json:"capability_version"`
}

func (i VerifiedIdentity) valid() bool {
	return i.Provider != "" && i.WorkspaceID != "" && i.CapabilityVersion != ""
}

// ConnectionSnapshot is the non-authorizing projection callers may persist.
// It intentionally excludes SessionRef, process-lifetime state, and the secret.
type ConnectionSnapshot struct {
	Kind     ConnectionKind   `json:"kind"`
	Identity VerifiedIdentity `json:"identity"`
}

// LeaseOptions deliberately has no wall-clock lifetime. A live lease belongs
// to this process and remains usable until an explicit revocation event occurs.
type LeaseOptions struct {
	Kind     ConnectionKind
	Secret   []byte
	Identity VerifiedIdentity
}

type RegistryOptions struct {
	Random             func([]byte) (int, error)
	MaximumSecretBytes int
}

type Registry struct {
	mu                 sync.Mutex
	random             func([]byte) (int, error)
	maximumSecretBytes int
	closed             bool
	entries            map[SessionRef]*leaseEntry
}

type leaseEntry struct {
	kind     ConnectionKind
	secret   []byte
	identity VerifiedIdentity
	revoked  bool
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewSessionRegistry(options RegistryOptions) *Registry {
	random := options.Random
	if random == nil {
		random = rand.Read
	}
	maximum := options.MaximumSecretBytes
	if maximum <= 0 {
		maximum = defaultMaximumSecretBytes
	}
	return &Registry{random: random, maximumSecretBytes: maximum, entries: map[SessionRef]*leaseEntry{}}
}

func (r *Registry) Register(options LeaseOptions) (SessionRef, ConnectionSnapshot, error) {
	if options.Kind == "" || len(options.Secret) == 0 || len(options.Secret) > r.maximumSecretBytes {
		return "", ConnectionSnapshot{}, ErrInvalidLeaseOptions
	}
	if options.Identity != (VerifiedIdentity{}) && !options.Identity.valid() {
		return "", ConnectionSnapshot{}, ErrInvalidLeaseOptions
	}
	ref, err := r.randomID(handleBytes)
	if err != nil {
		return "", ConnectionSnapshot{}, fmt.Errorf("create session reference: %w", err)
	}
	leaseContext, cancel := context.WithCancel(context.Background())
	entry := &leaseEntry{
		kind:     options.Kind,
		secret:   append([]byte(nil), options.Secret...),
		identity: options.Identity,
		ctx:      leaseContext,
		cancel:   cancel,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		cancel()
		zero(entry.secret)
		return "", ConnectionSnapshot{}, ErrRegistryClosed
	}
	if _, exists := r.entries[SessionRef(ref)]; exists {
		cancel()
		zero(entry.secret)
		return "", ConnectionSnapshot{}, errors.New("session reference collision")
	}
	r.entries[SessionRef(ref)] = entry
	return SessionRef(ref), entry.snapshot(), nil
}

// PinIdentity implements explicit trust-on-first-use and exact-match reconnect.
// Any observed identity drift terminally revokes the credential lease.
func (r *Registry) PinIdentity(ref SessionRef, identity VerifiedIdentity) (ConnectionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, err := r.usableLocked(ref)
	if err != nil {
		return ConnectionSnapshot{}, err
	}
	if !identity.valid() {
		r.revokeLocked(entry)
		return ConnectionSnapshot{}, ErrIdentityMismatch
	}
	if entry.identity == (VerifiedIdentity{}) {
		entry.identity = identity
	} else if !sameIdentity(entry.identity, identity) {
		r.revokeLocked(entry)
		return ConnectionSnapshot{}, ErrIdentityMismatch
	}
	return entry.snapshot(), nil
}

func (r *Registry) Snapshot(ref SessionRef) (ConnectionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, err := r.usableLocked(ref)
	if err != nil {
		return ConnectionSnapshot{}, err
	}
	return entry.snapshot(), nil
}

// Use is the only secret access boundary. It rechecks pinned identity
// immediately before every external call. Revocation, disconnect, provider
// rejection, identity drift, or shutdown cancels the callback context.
func (r *Registry) Use(ctx context.Context, ref SessionRef, expected VerifiedIdentity, call func(context.Context, []byte) error) error {
	if call == nil {
		return errors.New("missing lease callback")
	}
	r.mu.Lock()
	entry, err := r.usableLocked(ref)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if entry.identity == (VerifiedIdentity{}) {
		r.mu.Unlock()
		return ErrIdentityNotPinned
	}
	if !expected.valid() || !sameIdentity(entry.identity, expected) {
		r.revokeLocked(entry)
		r.mu.Unlock()
		return ErrIdentityMismatch
	}
	secret := append([]byte(nil), entry.secret...)
	leaseContext := entry.ctx
	r.mu.Unlock()
	defer zero(secret)

	callContext, cancel := mergeContexts(ctx, leaseContext)
	defer cancel()
	callErr := call(callContext, secret)
	if errors.Is(callErr, ErrCredentialRejected) {
		r.mu.Lock()
		// The reference is process-unique, but guard the entry identity so a
		// future implementation cannot revoke a replacement accidentally.
		if current, ok := r.entries[ref]; ok && current == entry {
			r.revokeLocked(current)
		}
		r.mu.Unlock()
	}
	return callErr
}

func (r *Registry) Revoke(ref SessionRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[ref]
	if !ok {
		return ErrLeaseNotFound
	}
	r.revokeLocked(entry)
	return nil
}

func (r *Registry) Disconnect(ref SessionRef) error { return r.Revoke(ref) }

func (r *Registry) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for _, entry := range r.entries {
		r.revokeLocked(entry)
	}
}

func (r *Registry) usableLocked(ref SessionRef) (*leaseEntry, error) {
	if r.closed {
		return nil, ErrRegistryClosed
	}
	entry, ok := r.entries[ref]
	if !ok {
		return nil, ErrLeaseNotFound
	}
	if entry.revoked {
		return nil, ErrLeaseRevoked
	}
	return entry, nil
}

func (r *Registry) revokeLocked(entry *leaseEntry) {
	if entry.revoked {
		return
	}
	entry.revoked = true
	entry.cancel()
	zero(entry.secret)
	entry.secret = nil
}

func (r *Registry) randomID(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	n, err := r.random(buffer)
	if err != nil {
		return "", err
	}
	if n != len(buffer) {
		return "", errors.New("short random read")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func (e *leaseEntry) snapshot() ConnectionSnapshot {
	return ConnectionSnapshot{Kind: e.kind, Identity: e.identity}
}

func sameIdentity(left, right VerifiedIdentity) bool {
	// Constant-time comparison is inexpensive and avoids teaching callers to
	// branch on individual provider identity fields.
	a := []byte(left.Provider + "\x00" + left.WorkspaceID + "\x00" + left.ChannelID + "\x00" + left.KeyID + "\x00" + left.CapabilityVersion)
	b := []byte(right.Provider + "\x00" + right.WorkspaceID + "\x00" + right.ChannelID + "\x00" + right.KeyID + "\x00" + right.CapabilityVersion)
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func mergeContexts(parent, lease context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(lease, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
