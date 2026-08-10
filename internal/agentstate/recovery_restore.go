package agentstate

import (
	"context"
	"errors"
)

func (store *Store) restoreRecoverySnapshot(ctx context.Context, snapshot recoverySnapshot) error {
	if validateRecoverySnapshot(snapshot) != nil {
		return errors.New("restore agent recovery snapshot")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("restore agent recovery snapshot")
	}
	defer tx.Rollback()
	for _, lens := range snapshot.Lenses {
		if !validBounded(lens.ID, 256) || !validBounded(lens.Name, 1024) ||
			!validBounded(lens.Query, maximumTextRunes) ||
			!validBounded(lens.CreatedAt, 256) || !validBounded(lens.UpdatedAt, 256) {
			return errors.New("restore agent recovery snapshot")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO lenses(
			id, name, query, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?)`,
			lens.ID, lens.Name, lens.Query, lens.CreatedAt, lens.UpdatedAt,
		); err != nil {
			return errors.New("restore agent recovery snapshot")
		}
	}
	originals := make(map[string]Judgment, len(snapshot.Judgments))
	for _, judgment := range snapshot.Judgments {
		if judgment.ReversesID == "" {
			if err := validateRecoveredOriginal(judgment); err != nil {
				return err
			}
			originals[judgment.JudgmentID] = judgment
		}
	}
	for _, judgment := range snapshot.Judgments {
		if judgment.ReversesID != "" {
			original, exists := originals[judgment.ReversesID]
			if !exists || validateRecoveredReversal(judgment, original) != nil {
				return errors.New("restore agent recovery snapshot")
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO judgments(
			judgment_id, idempotency_key, run_id, lens_id, record_id, actor,
			disposition, reason, reverses_judgment_id, effect, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			judgment.JudgmentID, judgment.IdempotencyKey, judgment.RunID,
			judgment.LensID, judgment.RecordID, judgment.Actor, judgment.Disposition,
			judgment.Reason, judgment.ReversesID, judgment.Effect, judgment.CreatedAt,
		); err != nil {
			return errors.New("restore agent recovery snapshot")
		}
	}
	return tx.Commit()
}

func validateRecoveredOriginal(judgment Judgment) error {
	if !validBounded(judgment.JudgmentID, 256) ||
		!validBounded(judgment.IdempotencyKey, 1024) ||
		!validBounded(judgment.RunID, 256) ||
		!validBounded(judgment.LensID, 256) ||
		!validBounded(judgment.RecordID, 1024) ||
		!validBounded(judgment.CreatedAt, 256) ||
		!validOptional(judgment.Reason, 4096) ||
		containsSecretLikeAny(judgment.JudgmentID, judgment.IdempotencyKey,
			judgment.RunID, judgment.LensID, judgment.RecordID, judgment.Reason,
			judgment.ReversesID, judgment.CreatedAt) ||
		judgment.ReversesID != "" {
		return errors.New("restore agent recovery snapshot")
	}
	expectedEffect, err := judgmentEffect(judgment.Actor, judgment.Disposition)
	if err != nil || judgment.Effect != expectedEffect {
		return errors.New("restore agent recovery snapshot")
	}
	return nil
}

func validateRecoveredReversal(judgment, original Judgment) error {
	if !validBounded(judgment.JudgmentID, 256) ||
		!validBounded(judgment.IdempotencyKey, 1024) ||
		!validBounded(judgment.RunID, 256) ||
		!validBounded(judgment.LensID, 256) ||
		!validBounded(judgment.RecordID, 1024) ||
		!validBounded(judgment.CreatedAt, 256) ||
		!validOptional(judgment.Reason, 4096) ||
		containsSecretLikeAny(judgment.JudgmentID, judgment.IdempotencyKey,
			judgment.RunID, judgment.LensID, judgment.RecordID, judgment.Reason,
			judgment.ReversesID, judgment.CreatedAt) ||
		(judgment.Actor != "user" && judgment.Actor != "agent") ||
		judgment.Disposition != "reversed" ||
		judgment.RunID != original.RunID ||
		judgment.LensID != original.LensID ||
		judgment.RecordID != original.RecordID ||
		judgment.Effect != -original.Effect {
		return errors.New("restore agent recovery snapshot")
	}
	return nil
}
