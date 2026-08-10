package agentstate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (store *Store) PutLens(ctx context.Context, lens Lens) (Lens, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	lens.ID = strings.TrimSpace(lens.ID)
	lens.Name = strings.TrimSpace(lens.Name)
	lens.Query = strings.TrimSpace(lens.Query)
	if !validBounded(lens.ID, 256) || !validBounded(lens.Name, 1024) || !validBounded(lens.Query, maximumTextRunes) ||
		containsSecretLikeAny(lens.ID, lens.Name, lens.Query) {
		return Lens{}, errors.New("invalid lens")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err := store.db.ExecContext(ctx, `INSERT INTO lenses(id, name, query, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, query=excluded.query, updated_at=excluded.updated_at`,
		lens.ID, lens.Name, lens.Query, now, now,
	)
	if err != nil {
		return Lens{}, errors.New("save lens")
	}
	saved, err := store.GetLens(ctx, lens.ID)
	if err != nil {
		return Lens{}, err
	}
	if err := store.writeRecoverySnapshot(ctx); err != nil {
		return Lens{}, err
	}
	return saved, nil
}

func (store *Store) GetLens(ctx context.Context, id string) (Lens, error) {
	var lens Lens
	err := store.db.QueryRowContext(ctx,
		`SELECT id, name, query, created_at, updated_at FROM lenses WHERE id=?`, strings.TrimSpace(id),
	).Scan(&lens.ID, &lens.Name, &lens.Query, &lens.CreatedAt, &lens.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Lens{}, errors.New("lens not found")
	}
	if err != nil {
		return Lens{}, errors.New("read lens")
	}
	return lens, nil
}

func (store *Store) ListLenses(ctx context.Context) ([]Lens, error) {
	rows, err := store.db.QueryContext(ctx,
		`SELECT id, name, query, created_at, updated_at FROM lenses ORDER BY id`,
	)
	if err != nil {
		return nil, errors.New("list lenses")
	}
	defer rows.Close()
	lenses := []Lens{}
	for rows.Next() {
		var lens Lens
		if err := rows.Scan(&lens.ID, &lens.Name, &lens.Query, &lens.CreatedAt, &lens.UpdatedAt); err != nil {
			return nil, errors.New("list lenses")
		}
		lenses = append(lenses, lens)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("list lenses")
	}
	return lenses, nil
}

func (store *Store) DeleteLens(ctx context.Context, id string) (bool, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	result, err := store.db.ExecContext(ctx, `DELETE FROM lenses WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return false, errors.New("delete lens")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := store.writeRecoverySnapshot(ctx); err != nil {
		return false, err
	}
	return count > 0, nil
}
