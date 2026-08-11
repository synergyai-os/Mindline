package agentstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/contentguard"
	"github.com/synergyai-os/Mindline/internal/privateio"
	_ "modernc.org/sqlite"
)

const (
	maximumTextRunes  = 64 << 10
	maximumVectorSize = 16_384
)

var (
	ErrCorrupt            = errors.New("agent state database is corrupt")
	ErrRecoveryInProgress = errors.New("agent state recovery is in progress")
)

type Store struct {
	db                         *sql.DB
	path                       string
	now                        Clock
	mutationMu                 sync.Mutex
	scopedRecoveryByteLimit    int64
	projectConnectionInitHook  func() error
	projectConnectionWriteHook func() error
}

func Open(path string, now Clock) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("agent state path must be absolute")
	}
	path = filepath.Clean(path)
	if recoveryMarkerExists(path) {
		return nil, ErrRecoveryInProgress
	}
	return openStore(path, now, true)
}

func openStore(path string, now Clock, snapshot bool) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("agent state path must be absolute")
	}
	path = filepath.Clean(path)
	root := filepath.Dir(path)
	if err := privateio.PrepareDir(root); err != nil {
		return nil, errors.New("prepare agent state: private root unavailable")
	}
	if err := privateio.ValidateContained(root, path); err != nil {
		return nil, errors.New("prepare agent state: unsafe path")
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, privateio.FileMode)
		if createErr != nil {
			return nil, errors.New("prepare agent state: database unavailable")
		}
		if closeErr := file.Close(); closeErr != nil {
			return nil, errors.New("prepare agent state: database unavailable")
		}
	} else if err != nil {
		return nil, errors.New("prepare agent state: database unavailable")
	}
	if err := requirePrivateDatabase(path); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.New("open agent state database")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, now: now}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if snapshot {
		if err := store.writeRecoverySnapshot(context.Background()); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return store, nil
}

func requirePrivateDatabase(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
		return errors.New("agent state database must be an owner-only regular file")
	}
	return nil
}

func (store *Store) initialize() error {
	var integrity string
	if err := store.db.QueryRow(`PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("%w: quick check failed", ErrCorrupt)
	}
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS state_meta (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS lenses (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			query TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			document_id TEXT PRIMARY KEY NOT NULL,
			document_fingerprint TEXT NOT NULL,
			model TEXT NOT NULL,
			dimensions INTEGER NOT NULL,
			vector_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS retrieval_runs (
			run_id TEXT PRIMARY KEY NOT NULL,
			query TEXT NOT NULL,
			lens_id TEXT NOT NULL,
			retrieval_method TEXT NOT NULL,
			library_fingerprint TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS retrieval_candidates (
			run_id TEXT NOT NULL REFERENCES retrieval_runs(run_id) ON DELETE CASCADE,
			rank INTEGER NOT NULL,
			record_id TEXT NOT NULL,
			final_score REAL NOT NULL,
			components_json BLOB NOT NULL,
			PRIMARY KEY (run_id, record_id)
		)`,
		`CREATE TABLE IF NOT EXISTS judgments (
			judgment_id TEXT PRIMARY KEY NOT NULL,
			idempotency_key TEXT UNIQUE NOT NULL,
			run_id TEXT NOT NULL,
			lens_id TEXT NOT NULL,
			record_id TEXT NOT NULL,
			actor TEXT NOT NULL,
			disposition TEXT NOT NULL,
			reason TEXT NOT NULL,
			reverses_judgment_id TEXT NOT NULL,
			effect REAL NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS judgments_one_reversal
			ON judgments(reverses_judgment_id) WHERE reverses_judgment_id <> ''`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize agent state: %w", err)
		}
	}
	if err := store.db.QueryRow(`PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("%w: quick check failed", ErrCorrupt)
	}
	if _, err := store.db.Exec(
		`INSERT INTO state_meta(key, value) VALUES('schema_version', ?)
			 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, SchemaVersion,
	); err != nil {
		return errors.New("initialize agent state schema")
	}
	if err := store.initializeScoped(context.Background()); err != nil {
		return err
	}
	if err := store.validateDurablePrivacy(context.Background()); err != nil {
		return err
	}
	return store.secureFiles()
}

func (store *Store) secureFiles() error {
	for _, path := range []string{store.path, store.path + "-wal", store.path + "-shm"} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return errors.New("secure agent state files")
		}
		if err := os.Chmod(path, privateio.FileMode); err != nil {
			return errors.New("secure agent state files")
		}
	}
	return nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	_, _ = store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	err := store.db.Close()
	if secureErr := store.secureFiles(); err == nil {
		err = secureErr
	}
	return err
}

func (store *Store) LoadEmbedding(ctx context.Context, documentID, fingerprint, model string) ([]float64, bool, error) {
	var data []byte
	var dimensions int
	err := store.db.QueryRowContext(ctx, `SELECT dimensions, vector_json FROM embeddings
		WHERE document_id=? AND document_fingerprint=? AND model=?`,
		documentID, fingerprint, model,
	).Scan(&dimensions, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("read embedding")
	}
	var vector []float64
	if err := json.Unmarshal(data, &vector); err != nil || len(vector) != dimensions || len(vector) == 0 || len(vector) > maximumVectorSize {
		if _, deleteErr := store.db.ExecContext(ctx, `DELETE FROM embeddings WHERE document_id=?`, documentID); deleteErr != nil {
			return nil, false, errors.New("rebuild invalid stored embedding")
		}
		return nil, false, nil
	}
	return vector, true, nil
}

func (store *Store) SaveEmbedding(ctx context.Context, embedding Embedding) error {
	if !validBounded(embedding.DocumentID, 1024) ||
		!validBounded(embedding.DocumentFingerprint, 256) ||
		!validBounded(embedding.Model, 512) ||
		containsSecretLikeAny(embedding.DocumentID, embedding.DocumentFingerprint, embedding.Model) ||
		len(embedding.Vector) == 0 || len(embedding.Vector) > maximumVectorSize {
		return errors.New("invalid embedding")
	}
	data, err := json.Marshal(embedding.Vector)
	if err != nil {
		return errors.New("encode embedding")
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO embeddings(
		document_id, document_fingerprint, model, dimensions, vector_json, updated_at
	) VALUES(?, ?, ?, ?, ?, ?)
	ON CONFLICT(document_id) DO UPDATE SET
		document_fingerprint=excluded.document_fingerprint,
		model=excluded.model,
		dimensions=excluded.dimensions,
		vector_json=excluded.vector_json,
		updated_at=excluded.updated_at`,
		embedding.DocumentID, embedding.DocumentFingerprint, embedding.Model,
		len(embedding.Vector), data, store.now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return errors.New("save embedding")
	}
	return nil
}

func (store *Store) SetIndexedFingerprint(ctx context.Context, fingerprint string) error {
	if !validBounded(fingerprint, 256) || contentguard.ContainsSecretLike(fingerprint) {
		return errors.New("invalid library fingerprint")
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO state_meta(key, value) VALUES('indexed_library_fingerprint', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fingerprint)
	return err
}

func (store *Store) SaveRetrieval(ctx context.Context, trace RetrievalTrace) error {
	if !validBounded(trace.RunID, 256) || !validBounded(trace.Query, maximumTextRunes) ||
		len(trace.Candidates) > 100 || !validBounded(trace.RetrievalMethod, 1024) ||
		!validBounded(trace.LibraryFingerprint, 256) || !validBounded(trace.CreatedAt, 256) ||
		containsSecretLikeAny(trace.RunID, trace.Query, trace.LensID, trace.RetrievalMethod,
			trace.LibraryFingerprint, trace.CreatedAt) {
		return errors.New("invalid retrieval trace")
	}
	for _, candidate := range trace.Candidates {
		if !validBounded(candidate.RecordID, 1024) || contentguard.ContainsSecretLike(candidate.RecordID) {
			return errors.New("invalid retrieval trace")
		}
		for component := range candidate.ComponentScore {
			if !validBounded(component, 256) || contentguard.ContainsSecretLike(component) {
				return errors.New("invalid retrieval trace")
			}
		}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("start retrieval trace")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_runs(
		run_id, query, lens_id, retrieval_method, library_fingerprint, created_at
	) VALUES(?, ?, ?, ?, ?, ?)`,
		trace.RunID, trace.Query, trace.LensID, trace.RetrievalMethod,
		trace.LibraryFingerprint, trace.CreatedAt,
	); err != nil {
		return errors.New("save retrieval trace")
	}
	for _, candidate := range trace.Candidates {
		components, err := json.Marshal(candidate.ComponentScore)
		if err != nil {
			return errors.New("encode retrieval trace")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_candidates(
			run_id, rank, record_id, final_score, components_json
		) VALUES(?, ?, ?, ?, ?)`,
			trace.RunID, candidate.Rank, candidate.RecordID, candidate.FinalScore, components,
		); err != nil {
			return errors.New("save retrieval candidate")
		}
	}
	return tx.Commit()
}

func (store *Store) Status(ctx context.Context) (Status, error) {
	status := Status{SchemaVersion: SchemaVersion, DatabasePath: store.path}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lenses`).Scan(&status.LensCount); err != nil {
		return Status{}, errors.New("read agent state status")
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM retrieval_runs`).Scan(&status.RetrievalRunCount); err != nil {
		return Status{}, errors.New("read agent state status")
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM judgments`).Scan(&status.JudgmentCount); err != nil {
		return Status{}, errors.New("read agent state status")
	}
	for query, destination := range map[string]*int{
		`SELECT COUNT(*) FROM scopes`:                                      &status.ScopeCount,
		`SELECT COUNT(*) FROM scoped_lenses`:                               &status.ScopedLensCount,
		`SELECT COUNT(*) FROM agent_actors`:                                &status.AgentActorCount,
		`SELECT COUNT(*) FROM scoped_retrieval_runs`:                       &status.ScopedRetrievalRunCount,
		`SELECT COUNT(*) FROM scoped_judgments`:                            &status.ScopedJudgmentCount,
		`SELECT COUNT(*) FROM project_connections`:                         &status.ProjectConnectionCount,
		`SELECT COUNT(*) FROM project_connections WHERE status='active'`:   &status.ActiveConnectionCount,
		`SELECT COUNT(*) FROM project_connections WHERE status='archived'`: &status.ArchivedConnectionCount,
	} {
		if err := store.db.QueryRowContext(ctx, query).Scan(destination); err != nil {
			return Status{}, errors.New("read agent state status")
		}
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM embeddings`).Scan(&status.EmbeddingCount); err != nil {
		return Status{}, errors.New("read agent state status")
	}
	_ = store.db.QueryRowContext(ctx,
		`SELECT value FROM state_meta WHERE key='indexed_library_fingerprint'`,
	).Scan(&status.IndexedFingerprint)
	return status, nil
}

func validBounded(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]rune(value)) <= maximum
}

func validOptional(value string, maximum int) bool {
	return strings.TrimSpace(value) == "" || len([]rune(value)) <= maximum
}

func containsSecretLikeAny(values ...string) bool {
	for _, value := range values {
		if contentguard.ContainsSecretLike(value) {
			return true
		}
	}
	return false
}
