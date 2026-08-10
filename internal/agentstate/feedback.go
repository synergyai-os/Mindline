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

	"github.com/synergyai-os/Mindline/internal/contentguard"
)

const maximumFeedbackRetryTokenRunes = 256

func (store *Store) ApplyJudgment(ctx context.Context, request JudgmentRequest) (Judgment, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.RunID = strings.TrimSpace(request.RunID)
	request.LensID = strings.TrimSpace(request.LensID)
	request.RecordID = strings.TrimSpace(request.RecordID)
	request.Actor = strings.TrimSpace(request.Actor)
	request.Disposition = strings.TrimSpace(request.Disposition)
	request.Reason = strings.TrimSpace(request.Reason)
	request.ReversesID = strings.TrimSpace(request.ReversesID)
	request.RetryToken = strings.TrimSpace(request.RetryToken)
	var retryPrefix string
	if request.RetryToken != "" {
		var err error
		request.IdempotencyKey, retryPrefix, err = feedbackRetryIdentity(request)
		if err != nil {
			return Judgment{}, err
		}
	} else if request.ReversesID == "" && !validBounded(request.IdempotencyKey, 1024) {
		return Judgment{}, errors.New("invalid judgment idempotency key")
	}
	if request.RetryToken != "" && strings.TrimSpace(request.IdempotencyKey) == "" {
		return Judgment{}, errors.New("invalid feedback retry identity")
	}
	if !validBounded(request.IdempotencyKey, 1024) {
		return Judgment{}, errors.New("invalid judgment idempotency key")
	}
	if (request.Actor != "user" && request.Actor != "agent") ||
		!validOptional(request.Reason, 4096) || contentguard.ContainsSecretLike(request.Reason) ||
		(request.ReversesID == "" &&
			(!validBounded(request.RunID, 256) ||
				!validBounded(request.LensID, 256) ||
				!validBounded(request.RecordID, 1024) ||
				!validBounded(request.Disposition, 64))) ||
		(request.ReversesID != "" &&
			(!validBounded(request.ReversesID, 256) ||
				request.RunID != "" || request.LensID != "" ||
				request.RecordID != "" || request.Disposition != "" ||
				request.RetryToken != "")) {
		return Judgment{}, errors.New("invalid judgment")
	}
	if retryPrefix != "" {
		existingKey, exists, err := store.judgmentKeyByRetryPrefix(ctx, retryPrefix)
		if err != nil {
			return Judgment{}, err
		}
		if exists && existingKey != request.IdempotencyKey {
			return Judgment{}, errors.New("feedback retry token conflicts with a different intent")
		}
	}
	if existing, exists, err := store.judgmentByIdempotency(ctx, request.IdempotencyKey); err != nil {
		return Judgment{}, err
	} else if exists {
		if !sameJudgmentRequest(existing, request) {
			return Judgment{}, errors.New("judgment idempotency key conflicts with a different request")
		}
		if err := store.writeRecoverySnapshot(ctx); err != nil {
			return Judgment{}, err
		}
		existing.Replayed = true
		return existing, nil
	}

	judgment := Judgment{
		JudgmentID:     stableID("judgment", request.IdempotencyKey),
		IdempotencyKey: request.IdempotencyKey,
		RunID:          request.RunID,
		LensID:         request.LensID,
		RecordID:       request.RecordID,
		Actor:          request.Actor,
		Disposition:    request.Disposition,
		Reason:         request.Reason,
		ReversesID:     request.ReversesID,
		CreatedAt:      store.now().UTC().Format(time.RFC3339Nano),
	}
	if judgment.ReversesID != "" {
		original, err := store.judgmentByID(ctx, judgment.ReversesID)
		if err != nil {
			return Judgment{}, err
		}
		if original.ReversesID != "" {
			return Judgment{}, errors.New("a reversal cannot reverse another reversal")
		}
		judgment.RunID = original.RunID
		judgment.LensID = original.LensID
		judgment.RecordID = original.RecordID
		judgment.Actor = request.Actor
		judgment.Disposition = "reversed"
		judgment.Effect = -original.Effect
	} else {
		runLens, err := store.retrievalCandidateLens(ctx, judgment.RunID, judgment.RecordID)
		if err != nil {
			return Judgment{}, err
		}
		if runLens != judgment.LensID {
			return Judgment{}, errors.New("judgment lens does not match retrieval run")
		}
		effect, err := judgmentEffect(judgment.Actor, judgment.Disposition)
		if err != nil || !validBounded(judgment.RecordID, 1024) {
			return Judgment{}, errors.New("invalid judgment")
		}
		judgment.Effect = effect
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO judgments(
		judgment_id, idempotency_key, run_id, lens_id, record_id, actor,
		disposition, reason, reverses_judgment_id, effect, created_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		judgment.JudgmentID, judgment.IdempotencyKey, judgment.RunID,
		judgment.LensID, judgment.RecordID, judgment.Actor, judgment.Disposition,
		judgment.Reason, judgment.ReversesID, judgment.Effect, judgment.CreatedAt,
	)
	if err != nil {
		return Judgment{}, errors.New("save judgment")
	}
	if err := store.writeRecoverySnapshot(ctx); err != nil {
		return Judgment{}, err
	}
	return judgment, nil
}

func FeedbackIdempotencyKey(request JudgmentRequest) (string, error) {
	key, _, err := feedbackRetryIdentity(request)
	return key, err
}

func feedbackRetryIdentity(request JudgmentRequest) (string, string, error) {
	request.RetryToken = strings.TrimSpace(request.RetryToken)
	request.RunID = strings.TrimSpace(request.RunID)
	request.LensID = strings.TrimSpace(request.LensID)
	request.RecordID = strings.TrimSpace(request.RecordID)
	request.Actor = strings.TrimSpace(request.Actor)
	request.Disposition = strings.TrimSpace(request.Disposition)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.IdempotencyKey != "" || request.ReversesID != "" ||
		!validBounded(request.RetryToken, maximumFeedbackRetryTokenRunes) ||
		!validBounded(request.RunID, 256) || !validBounded(request.LensID, 256) ||
		!validBounded(request.RecordID, 1024) ||
		(request.Actor != "user" && request.Actor != "agent") ||
		(request.Disposition != "used" && request.Disposition != "dismissed") ||
		!validOptional(request.Reason, 4096) {
		return "", "", errors.New("invalid feedback retry identity")
	}
	reasonHash := sha256.Sum256([]byte(request.Reason))
	payload, err := json.Marshal([]string{
		"v1", request.RunID, request.LensID, request.RecordID, request.Actor,
		request.Disposition, hex.EncodeToString(reasonHash[:]), request.RetryToken,
	})
	if err != nil {
		return "", "", errors.New("derive feedback retry identity")
	}
	eventHash := sha256.Sum256(payload)
	tokenHash := sha256.Sum256([]byte(request.RetryToken))
	prefix := "feedback-v1:" + hex.EncodeToString(tokenHash[:]) + ":"
	return prefix + hex.EncodeToString(eventHash[:]), prefix, nil
}

func sameJudgmentRequest(existing Judgment, request JudgmentRequest) bool {
	if existing.IdempotencyKey != request.IdempotencyKey ||
		existing.Actor != request.Actor || existing.Reason != request.Reason {
		return false
	}
	if request.ReversesID != "" {
		return existing.ReversesID == request.ReversesID
	}
	return existing.ReversesID == "" &&
		existing.RunID == request.RunID &&
		existing.LensID == request.LensID &&
		existing.RecordID == request.RecordID &&
		existing.Disposition == request.Disposition
}

func (store *Store) retrievalCandidateLens(ctx context.Context, runID, recordID string) (string, error) {
	if !validBounded(runID, 256) || !validBounded(recordID, 1024) {
		return "", errors.New("judgment must reference a retrieval candidate")
	}
	var lensID string
	err := store.db.QueryRowContext(ctx, `SELECT retrieval_runs.lens_id
		FROM retrieval_candidates
		JOIN retrieval_runs ON retrieval_runs.run_id=retrieval_candidates.run_id
		WHERE retrieval_candidates.run_id=? AND retrieval_candidates.record_id=?`,
		runID, recordID,
	).Scan(&lensID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("judgment must reference a retrieval candidate")
	}
	if err != nil {
		return "", errors.New("read retrieval candidate")
	}
	return lensID, nil
}

func judgmentEffect(actor, disposition string) (float64, error) {
	weight := 0.0
	switch actor {
	case "user":
		weight = 1
	case "agent":
		weight = 0.25
	default:
		return 0, errors.New("invalid judgment actor")
	}
	switch disposition {
	case "used":
		return weight, nil
	case "dismissed":
		return -weight, nil
	default:
		return 0, errors.New("invalid judgment disposition")
	}
}

func (store *Store) Relevance(ctx context.Context, lensID string, recordIDs []string) (map[string]float64, error) {
	result := make(map[string]float64, len(recordIDs))
	if len(recordIDs) == 0 || strings.TrimSpace(lensID) == "" {
		return result, nil
	}
	if len(recordIDs) > 100_000 {
		return nil, errors.New("relevance request exceeds limit")
	}
	rows, err := store.db.QueryContext(ctx, `SELECT record_id, SUM(effect)
		FROM judgments WHERE lens_id=? GROUP BY record_id`, lensID)
	if err != nil {
		return nil, errors.New("read relevance")
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
			return nil, errors.New("read relevance")
		}
		if allowed[recordID] {
			result[recordID] = clamp(value*0.1, -0.3, 0.3)
		}
	}
	return result, rows.Err()
}

func (store *Store) judgmentByIdempotency(ctx context.Context, key string) (Judgment, bool, error) {
	row := store.db.QueryRowContext(ctx, `SELECT judgment_id, idempotency_key, run_id,
		lens_id, record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM judgments WHERE idempotency_key=?`, key)
	judgment, err := scanJudgment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Judgment{}, false, nil
	}
	if err != nil {
		return Judgment{}, false, errors.New("read judgment")
	}
	return judgment, true, nil
}

func (store *Store) judgmentKeyByRetryPrefix(ctx context.Context, prefix string) (string, bool, error) {
	var key string
	err := store.db.QueryRowContext(ctx,
		`SELECT idempotency_key FROM judgments
		WHERE idempotency_key LIKE ? ORDER BY created_at, judgment_id LIMIT 1`,
		prefix+"%",
	).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("read feedback retry identity")
	}
	return key, true, nil
}

func (store *Store) judgmentByID(ctx context.Context, id string) (Judgment, error) {
	row := store.db.QueryRowContext(ctx, `SELECT judgment_id, idempotency_key, run_id,
		lens_id, record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM judgments WHERE judgment_id=?`, id)
	judgment, err := scanJudgment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Judgment{}, errors.New("judgment not found")
	}
	if err != nil {
		return Judgment{}, errors.New("read judgment")
	}
	return judgment, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanJudgment(row rowScanner) (Judgment, error) {
	var judgment Judgment
	err := row.Scan(&judgment.JudgmentID, &judgment.IdempotencyKey, &judgment.RunID,
		&judgment.LensID, &judgment.RecordID, &judgment.Actor, &judgment.Disposition,
		&judgment.Reason, &judgment.ReversesID, &judgment.Effect, &judgment.CreatedAt)
	return judgment, err
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:16])
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
