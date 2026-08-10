package agentstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/synergyai-os/Mindline/internal/contentguard"
)

var legacyDurableTextQueries = []struct {
	name  string
	query string
}{
	{"legacy metadata", `SELECT key, value FROM state_meta`},
	{"legacy lenses", `SELECT id, name, query, created_at, updated_at FROM lenses`},
	{"legacy embeddings", `SELECT document_id, document_fingerprint, model, updated_at FROM embeddings`},
	{"legacy retrieval runs", `SELECT run_id, query, lens_id, retrieval_method, library_fingerprint, created_at FROM retrieval_runs`},
	{"legacy retrieval candidates", `SELECT run_id, record_id, CAST(components_json AS TEXT) FROM retrieval_candidates`},
	{"legacy judgments", `SELECT judgment_id, idempotency_key, run_id, lens_id, record_id, actor, disposition, reason, reverses_judgment_id, created_at FROM judgments`},
}

var scopedDurableTextQueries = []struct {
	name  string
	query string
}{
	{"scoped metadata", `SELECT key, value FROM scoped_meta`},
	{"scopes", `SELECT id, name, purpose, status, created_at, updated_at FROM scopes`},
	{"scoped lenses", `SELECT scope_id, id, name, query, status, created_at, updated_at FROM scoped_lenses`},
	{"agent actors", `SELECT id, name, status, created_at, updated_at FROM agent_actors`},
	{"scoped retrieval runs", `SELECT run_id, query, scope_id, lens_id, agent_id, retrieval_method, library_fingerprint, created_at FROM scoped_retrieval_runs`},
	{"scoped retrieval candidates", `SELECT run_id, record_id, CAST(components_json AS TEXT) FROM scoped_retrieval_candidates`},
	{"scoped judgments", `SELECT judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id, record_id, actor, disposition, reason, reverses_judgment_id, created_at FROM scoped_judgments`},
}

func (store *Store) validateLegacyProjectionPrivacy(ctx context.Context) error {
	return store.validateDurableTextQueries(ctx, legacyDurableTextQueries)
}

func (store *Store) validateDurablePrivacy(ctx context.Context) error {
	if err := store.validateDurableTextQueries(ctx, legacyDurableTextQueries); err != nil {
		return err
	}
	return store.validateDurableTextQueries(ctx, scopedDurableTextQueries)
}

func (store *Store) validateDurableTextQueries(ctx context.Context, checks []struct {
	name  string
	query string
}) error {
	for _, check := range checks {
		rows, err := store.db.QueryContext(ctx, check.query)
		if err != nil {
			return errors.New("validate durable agent state privacy")
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return errors.New("validate durable agent state privacy")
		}
		values := make([]sql.RawBytes, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		for rows.Next() {
			if err := rows.Scan(destinations...); err != nil {
				rows.Close()
				return errors.New("validate durable agent state privacy")
			}
			for _, value := range values {
				if contentguard.ContainsSecretLike(string(value)) {
					rows.Close()
					return errors.New("durable agent state contains credential-shaped " + check.name)
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return errors.New("validate durable agent state privacy")
		}
		if err := rows.Close(); err != nil {
			return errors.New("validate durable agent state privacy")
		}
	}
	return nil
}
