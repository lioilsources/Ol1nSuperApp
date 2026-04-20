package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

const (
	StatusPendingPayment = "pending_payment"
	StatusUploading      = "uploading"
	StatusReady          = "ready"
	StatusNotarized      = "notarized"
	StatusFailed         = "failed"
)

const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

const (
	JobTypeUpload   = "upload"
	JobTypeDownload = "download"
)

type Artifact struct {
	ID            string
	Filename      string
	ContentType   string
	SizeBytes     int64
	NZBPath       sql.NullString
	Status        string
	PaymentID     sql.NullString
	PaymentStatus sql.NullString
	TxHash        sql.NullString
	CreatedAt     int64
	UpdatedAt     int64
}

type Job struct {
	ID         string
	ArtifactID string
	Type       string
	Status     string
	Progress   int
	ErrorMsg   sql.NullString
	ResultURL  sql.NullString
	TmpPath    sql.NullString
	CreatedAt  int64
	UpdatedAt  int64
}

type DownloadToken struct {
	Token      string
	ArtifactID string
	ExpiresAt  int64
	CreatedAt  int64
}

type Queries struct {
	db *sql.DB
}

func New(d *sql.DB) *Queries {
	return &Queries{db: d}
}

func (q *Queries) DB() *sql.DB { return q.db }

// --- artifacts ---

func (q *Queries) InsertArtifactPending(ctx context.Context, id, filename, contentType string, size int64, paymentID string) error {
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO artifacts(id, filename, content_type, size_bytes, status, payment_id)
		VALUES(?, ?, ?, ?, 'pending_payment', ?)
	`, id, filename, contentType, size, paymentID)
	return err
}

func (q *Queries) GetArtifact(ctx context.Context, id string) (*Artifact, error) {
	var a Artifact
	err := q.db.QueryRowContext(ctx, `
		SELECT id, filename, content_type, size_bytes, nzb_path, status,
		       payment_id, payment_status, tx_hash, created_at, updated_at
		  FROM artifacts WHERE id = ?
	`, id).Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.NZBPath, &a.Status,
		&a.PaymentID, &a.PaymentStatus, &a.TxHash, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (q *Queries) GetArtifactByPaymentID(ctx context.Context, paymentID string) (*Artifact, error) {
	var a Artifact
	err := q.db.QueryRowContext(ctx, `
		SELECT id, filename, content_type, size_bytes, nzb_path, status,
		       payment_id, payment_status, tx_hash, created_at, updated_at
		  FROM artifacts WHERE payment_id = ?
	`, paymentID).Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.NZBPath, &a.Status,
		&a.PaymentID, &a.PaymentStatus, &a.TxHash, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (q *Queries) ListReadyArtifacts(ctx context.Context, limit int) ([]Artifact, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, filename, content_type, size_bytes, nzb_path, status,
		       payment_id, payment_status, tx_hash, created_at, updated_at
		  FROM artifacts WHERE status IN ('ready','notarized') ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.NZBPath, &a.Status,
			&a.PaymentID, &a.PaymentStatus, &a.TxHash, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkPaymentConfirmed atomically transitions artifact from pending_payment → uploading.
// Returns true when a transition happened (i.e. idempotent — only the first call wins).
func (q *Queries) MarkPaymentConfirmed(ctx context.Context, paymentID, paymentStatus string) (bool, error) {
	res, err := q.db.ExecContext(ctx, `
		UPDATE artifacts
		   SET status = 'uploading',
		       payment_status = ?,
		       updated_at = unixepoch()
		 WHERE payment_id = ? AND status = 'pending_payment'
	`, paymentStatus, paymentID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (q *Queries) UpdateArtifactPaymentStatus(ctx context.Context, paymentID, paymentStatus string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE artifacts SET payment_status = ?, updated_at = unixepoch()
		 WHERE payment_id = ?
	`, paymentStatus, paymentID)
	return err
}

// PromoteArtifactToReady updates status to ready, rewrites id to the content-hash,
// and sets nzb_path. Must be run inside a tx because id is the primary key.
func (q *Queries) PromoteArtifactToReady(ctx context.Context, oldID, newID, nzbPath string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET artifact_id = ? WHERE artifact_id = ?`, newID, oldID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE artifacts SET id = ?, nzb_path = ?, status = 'ready', updated_at = unixepoch() WHERE id = ?`,
		newID, nzbPath, oldID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queries) MarkArtifactFailed(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE artifacts SET status='failed', updated_at=unixepoch() WHERE id = ?`, id)
	return err
}

func (q *Queries) MarkArtifactNotarized(ctx context.Context, id, txHash string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE artifacts SET status='notarized', tx_hash=?, updated_at=unixepoch() WHERE id = ?
	`, txHash, id)
	return err
}

// --- jobs ---

func (q *Queries) InsertJob(ctx context.Context, id, artifactID, jobType, tmpPath string) error {
	var tmp sql.NullString
	if tmpPath != "" {
		tmp = sql.NullString{String: tmpPath, Valid: true}
	}
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO jobs(id, artifact_id, type, status, tmp_path)
		VALUES(?, ?, ?, 'queued', ?)
	`, id, artifactID, jobType, tmp)
	return err
}

func (q *Queries) GetJob(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := q.db.QueryRowContext(ctx, `
		SELECT id, artifact_id, type, status, progress, error_msg, result_url, tmp_path, created_at, updated_at
		  FROM jobs WHERE id = ?
	`, id).Scan(&j.ID, &j.ArtifactID, &j.Type, &j.Status, &j.Progress, &j.ErrorMsg, &j.ResultURL, &j.TmpPath, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (q *Queries) UpdateJobStatus(ctx context.Context, id, status string, progress int) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, progress=?, updated_at=unixepoch() WHERE id = ?`,
		status, progress, id)
	return err
}

func (q *Queries) UpdateJobArtifactID(ctx context.Context, id, artifactID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET artifact_id=?, updated_at=unixepoch() WHERE id = ?`,
		artifactID, id)
	return err
}

func (q *Queries) UpdateJobProgress(ctx context.Context, id string, progress int) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET progress=?, updated_at=unixepoch() WHERE id = ?`,
		progress, id)
	return err
}

func (q *Queries) MarkJobDone(ctx context.Context, id, resultURL string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET status='done', progress=100, result_url=?, updated_at=unixepoch() WHERE id = ?`,
		resultURL, id)
	return err
}

func (q *Queries) MarkJobFailed(ctx context.Context, id, msg string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET status='failed', error_msg=?, updated_at=unixepoch() WHERE id = ?`,
		msg, id)
	return err
}

// --- tokens ---

func (q *Queries) InsertDownloadToken(ctx context.Context, token, artifactID string, ttl time.Duration) error {
	expires := time.Now().Add(ttl).Unix()
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO download_tokens(token, artifact_id, expires_at) VALUES(?, ?, ?)`,
		token, artifactID, expires)
	return err
}

func (q *Queries) GetDownloadToken(ctx context.Context, token string) (*DownloadToken, error) {
	var t DownloadToken
	err := q.db.QueryRowContext(ctx, `
		SELECT token, artifact_id, expires_at, created_at FROM download_tokens WHERE token = ?
	`, token).Scan(&t.Token, &t.ArtifactID, &t.ExpiresAt, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (q *Queries) DeleteExpiredTokens(ctx context.Context, now int64) ([]DownloadToken, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT token, artifact_id, expires_at, created_at FROM download_tokens WHERE expires_at < ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var expired []DownloadToken
	for rows.Next() {
		var t DownloadToken
		if err := rows.Scan(&t.Token, &t.ArtifactID, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		expired = append(expired, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := q.db.ExecContext(ctx, `DELETE FROM download_tokens WHERE expires_at < ?`, now); err != nil {
		return nil, err
	}
	return expired, nil
}
