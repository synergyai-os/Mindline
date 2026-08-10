package agentstate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (store *Store) ApplyScopedJudgment(ctx context.Context, request ScopedJudgmentRequest) (ScopedJudgment, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	request = normalizeScopedJudgmentRequest(request)
	if err := validateScopedJudgmentRequest(request); err != nil {
		return ScopedJudgment{}, err
	}
	retryPrefix := ""
	if request.RetryToken != "" {
		key, prefix, err := scopedFeedbackRetryIdentity(request)
		if err != nil {
			return ScopedJudgment{}, err
		}
		request.IdempotencyKey, retryPrefix = key, prefix
	}
	if retryPrefix != "" {
		existingKey, exists, err := store.scopedJudgmentKeyByRetryPrefix(ctx, retryPrefix)
		if err != nil {
			return ScopedJudgment{}, err
		}
		if exists && existingKey != request.IdempotencyKey {
			return ScopedJudgment{}, errors.New("scoped feedback retry token conflicts with a different intent")
		}
	}
	if existing, exists, err := store.scopedJudgmentByIdempotency(ctx, request.IdempotencyKey); err != nil {
		return ScopedJudgment{}, err
	} else if exists {
		if !sameScopedJudgmentRequest(existing, request) {
			return ScopedJudgment{}, errors.New("scoped judgment idempotency key conflicts with a different request")
		}
		// The database insert can outlive a failed sidecar refresh. Repair the
		// recovery copy before acknowledging a retry so a later database recovery
		// cannot lose feedback the caller was told was durable.
		if err := store.writeRecoverySnapshot(ctx); err != nil {
			return ScopedJudgment{}, err
		}
		existing.Replayed = true
		return existing, nil
	}

	judgment := ScopedJudgment{
		JudgmentID:     stableID("scoped-judgment", request.IdempotencyKey),
		IdempotencyKey: request.IdempotencyKey, RunID: request.RunID,
		ScopeID: request.ScopeID, LensID: request.LensID, AgentID: request.AgentID,
		RecordID: request.RecordID, Actor: request.Actor, Disposition: request.Disposition,
		Reason: request.Reason, ReversesID: request.ReversesID,
		CreatedAt: store.now().UTC().Format(time.RFC3339Nano),
	}
	if request.ReversesID != "" {
		original, err := store.scopedJudgmentByID(ctx, request.ReversesID)
		if err != nil {
			return ScopedJudgment{}, err
		}
		if original.ReversesID != "" || original.Actor != request.Actor ||
			original.ScopeID != request.ScopeID || original.LensID != request.LensID ||
			original.AgentID != request.AgentID {
			return ScopedJudgment{}, errors.New("scoped reversal target does not match feedback context")
		}
		if _, exists, err := store.scopedReversalFor(ctx, original.JudgmentID); err != nil {
			return ScopedJudgment{}, err
		} else if exists {
			return ScopedJudgment{}, errors.New("scoped judgment is already reversed")
		}
		judgment.RunID = original.RunID
		judgment.RecordID = original.RecordID
		judgment.Disposition = "reversed"
		judgment.Effect = -original.Effect
		if err := store.requireActiveScopedRun(ctx, original.RunID, original.ScopeID,
			original.LensID, original.AgentID, original.Actor, original.RecordID); err != nil {
			return ScopedJudgment{}, err
		}
	} else {
		if err := store.requireActiveScopedRun(ctx, request.RunID, request.ScopeID,
			request.LensID, request.AgentID, request.Actor, request.RecordID); err != nil {
			return ScopedJudgment{}, err
		}
		effect, err := scopedJudgmentEffect(request.Actor, request.Disposition)
		if err != nil {
			return ScopedJudgment{}, err
		}
		judgment.Effect = effect
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO scoped_judgments(
		judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id,
		record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, judgment.JudgmentID,
		judgment.IdempotencyKey, judgment.RunID, judgment.ScopeID, judgment.LensID,
		judgment.AgentID, judgment.RecordID, judgment.Actor, judgment.Disposition,
		judgment.Reason, judgment.ReversesID, judgment.Effect, judgment.CreatedAt)
	if err != nil {
		return ScopedJudgment{}, errors.New("save scoped judgment")
	}
	if err := store.writeRecoverySnapshot(ctx); err != nil {
		return ScopedJudgment{}, err
	}
	return judgment, nil
}

func normalizeScopedJudgmentRequest(request ScopedJudgmentRequest) ScopedJudgmentRequest {
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.RetryToken = strings.TrimSpace(request.RetryToken)
	request.RunID = strings.TrimSpace(request.RunID)
	request.ScopeID = strings.TrimSpace(request.ScopeID)
	request.LensID = strings.TrimSpace(request.LensID)
	request.AgentID = strings.TrimSpace(request.AgentID)
	request.RecordID = strings.TrimSpace(request.RecordID)
	request.Actor = strings.TrimSpace(request.Actor)
	request.Disposition = strings.TrimSpace(request.Disposition)
	request.Reason = strings.TrimSpace(request.Reason)
	request.ReversesID = strings.TrimSpace(request.ReversesID)
	return request
}

func validateScopedJudgmentRequest(request ScopedJudgmentRequest) error {
	if !validBounded(request.ScopeID, 256) || !validBounded(request.LensID, 256) ||
		(request.Actor != FeedbackOwner && request.Actor != FeedbackAgent) ||
		(request.Actor == FeedbackOwner && request.AgentID != "") ||
		(request.Actor == FeedbackAgent && !validBounded(request.AgentID, 256)) ||
		!validOptional(request.Reason, 4096) ||
		containsSecretLikeAny(request.IdempotencyKey, request.RunID, request.ScopeID,
			request.LensID, request.AgentID, request.RecordID, request.Reason, request.ReversesID) {
		return errors.New("invalid scoped judgment")
	}
	if request.ReversesID != "" {
		if !validBounded(request.IdempotencyKey, 1024) || request.RetryToken != "" ||
			!validBounded(request.ReversesID, 256) || request.RunID != "" ||
			request.RecordID != "" || request.Disposition != "" {
			return errors.New("invalid scoped judgment reversal")
		}
		return nil
	}
	if (request.IdempotencyKey == "") == (request.RetryToken == "") ||
		!validBounded(request.RunID, 256) || !validBounded(request.RecordID, 1024) ||
		(request.Disposition != "used" && request.Disposition != "dismissed") ||
		(request.IdempotencyKey != "" && !validBounded(request.IdempotencyKey, 1024)) ||
		(request.RetryToken != "" && !validBounded(request.RetryToken, maximumFeedbackRetryTokenRunes)) {
		return errors.New("invalid scoped judgment")
	}
	return nil
}

func scopedFeedbackRetryIdentity(request ScopedJudgmentRequest) (string, string, error) {
	if request.IdempotencyKey != "" || request.ReversesID != "" {
		return "", "", errors.New("invalid scoped feedback retry identity")
	}
	reasonHash := sha256.Sum256([]byte(request.Reason))
	payload, err := json.Marshal([]string{"v2", request.RunID, request.ScopeID,
		request.LensID, request.AgentID, request.RecordID, request.Actor,
		request.Disposition, hex.EncodeToString(reasonHash[:]), request.RetryToken})
	if err != nil {
		return "", "", errors.New("derive scoped feedback retry identity")
	}
	eventHash := sha256.Sum256(payload)
	tokenHash := sha256.Sum256([]byte(request.RetryToken))
	prefix := "scoped-feedback-v2:" + hex.EncodeToString(tokenHash[:]) + ":"
	return prefix + hex.EncodeToString(eventHash[:]), prefix, nil
}

func (store *Store) requireActiveScopedRun(
	ctx context.Context,
	runID, scopeID, lensID, agentID, actor, recordID string,
) error {
	var runScope, runLens, runAgent string
	err := store.db.QueryRowContext(ctx, `SELECT r.scope_id, r.lens_id, r.agent_id
		FROM scoped_retrieval_runs r JOIN scoped_retrieval_candidates c ON c.run_id=r.run_id
		WHERE r.run_id=? AND c.record_id=?`, runID, recordID).Scan(&runScope, &runLens, &runAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("scoped judgment must reference a retrieval candidate")
	}
	if err != nil {
		return errors.New("read scoped retrieval candidate")
	}
	if runScope != scopeID || runLens != lensID ||
		(actor == FeedbackAgent && runAgent != agentID) {
		return errors.New("scoped judgment context does not match retrieval run")
	}
	_, _, _, err = store.resolveScopedContext(ctx, ScopedContext{
		ScopeID: runScope, LensID: runLens, AgentID: runAgent,
	})
	return err
}

func scopedJudgmentEffect(actor, disposition string) (float64, error) {
	weight := 1.0
	if actor == FeedbackAgent {
		weight = 0.25
	} else if actor != FeedbackOwner {
		return 0, errors.New("invalid scoped judgment actor")
	}
	if disposition == "used" {
		return weight, nil
	}
	if disposition == "dismissed" {
		return -weight, nil
	}
	return 0, errors.New("invalid scoped judgment disposition")
}

func (store *Store) ScopedRelevance(ctx context.Context, input ScopedContext, recordIDs []string) (map[string]float64, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	result := make(map[string]float64, len(recordIDs))
	if len(recordIDs) == 0 {
		return result, nil
	}
	if len(recordIDs) > 100_000 {
		return nil, errors.New("scoped relevance request exceeds limit")
	}
	if _, _, _, err := store.resolveScopedContext(ctx, input); err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, `SELECT record_id, SUM(effect)
		FROM scoped_judgments WHERE scope_id=? AND lens_id=? AND
		((actor='owner' AND agent_id='') OR (actor='agent' AND agent_id=?))
		GROUP BY record_id`, input.ScopeID, input.LensID, input.AgentID)
	if err != nil {
		return nil, errors.New("read scoped relevance")
	}
	defer rows.Close()
	allowed := make(map[string]bool, len(recordIDs))
	for _, id := range recordIDs {
		allowed[id] = true
	}
	for rows.Next() {
		var recordID string
		var value float64
		if err := rows.Scan(&recordID, &value); err != nil {
			return nil, errors.New("read scoped relevance")
		}
		if allowed[recordID] {
			result[recordID] = clamp(value*0.1, -0.3, 0.3)
		}
	}
	return result, rows.Err()
}

func sameScopedJudgmentRequest(existing ScopedJudgment, request ScopedJudgmentRequest) bool {
	if existing.IdempotencyKey != request.IdempotencyKey || existing.Actor != request.Actor ||
		existing.ScopeID != request.ScopeID || existing.LensID != request.LensID ||
		existing.AgentID != request.AgentID || existing.Reason != request.Reason {
		return false
	}
	if request.ReversesID != "" {
		return existing.ReversesID == request.ReversesID
	}
	return existing.ReversesID == "" && existing.RunID == request.RunID &&
		existing.RecordID == request.RecordID && existing.Disposition == request.Disposition
}

func (store *Store) scopedJudgmentByIdempotency(ctx context.Context, key string) (ScopedJudgment, bool, error) {
	judgment, err := scanScopedJudgment(store.db.QueryRowContext(ctx, `SELECT
		judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id,
		record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM scoped_judgments WHERE idempotency_key=?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return ScopedJudgment{}, false, nil
	}
	if err != nil {
		return ScopedJudgment{}, false, errors.New("read scoped judgment")
	}
	return judgment, true, nil
}

func (store *Store) scopedJudgmentByID(ctx context.Context, id string) (ScopedJudgment, error) {
	judgment, err := scanScopedJudgment(store.db.QueryRowContext(ctx, `SELECT
		judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id,
		record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM scoped_judgments WHERE judgment_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ScopedJudgment{}, errors.New("scoped judgment not found")
	}
	if err != nil {
		return ScopedJudgment{}, errors.New("read scoped judgment")
	}
	return judgment, nil
}

func (store *Store) scopedJudgmentKeyByRetryPrefix(ctx context.Context, prefix string) (string, bool, error) {
	var key string
	err := store.db.QueryRowContext(ctx, `SELECT idempotency_key FROM scoped_judgments
		WHERE idempotency_key LIKE ? ORDER BY created_at, judgment_id LIMIT 1`, prefix+"%").Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("read scoped feedback retry identity")
	}
	return key, true, nil
}

func (store *Store) scopedReversalFor(ctx context.Context, id string) (ScopedJudgment, bool, error) {
	judgment, err := scanScopedJudgment(store.db.QueryRowContext(ctx, `SELECT
		judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id,
		record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM scoped_judgments WHERE reverses_judgment_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ScopedJudgment{}, false, nil
	}
	if err != nil {
		return ScopedJudgment{}, false, errors.New("read scoped judgment reversal")
	}
	return judgment, true, nil
}

func scanScopedJudgment(row rowScanner) (ScopedJudgment, error) {
	var judgment ScopedJudgment
	err := row.Scan(&judgment.JudgmentID, &judgment.IdempotencyKey, &judgment.RunID,
		&judgment.ScopeID, &judgment.LensID, &judgment.AgentID, &judgment.RecordID,
		&judgment.Actor, &judgment.Disposition, &judgment.Reason, &judgment.ReversesID,
		&judgment.Effect, &judgment.CreatedAt)
	return judgment, err
}
