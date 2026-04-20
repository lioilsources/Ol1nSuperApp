package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/nntp"
)

type UploadJob struct {
	JobID      string
	ArtifactID string // initial id (will be replaced by content hash of the NZB)
	Filename   string
	TmpPath    string
	NZBDir     string
	Poster     string
	Newsgroup  string

	Client *nntp.Client
	Worker *Worker
}

func (j *UploadJob) ID() string { return j.JobID }

func (j *UploadJob) Run(ctx context.Context) error {
	data, err := os.ReadFile(j.TmpPath)
	if err != nil {
		return fmt.Errorf("read tmp: %w", err)
	}
	totalSize := int64(len(data))
	fileCRC := crc32.ChecksumIEEE(data)

	parts, offsets := nntp.SplitSegments(data, nntp.SegmentSize)
	totalParts := len(parts)
	if totalParts == 0 {
		return fmt.Errorf("upload: empty file")
	}

	segments := make([]nntp.NZBSegment, 0, totalParts)
	for i, part := range parts {
		partNum := i + 1
		body := nntp.YEncEncodeSegment(j.Filename, partNum, totalParts, totalSize, offsets[i], part, fileCRC)
		subject := fmt.Sprintf("[1/1] %s yEnc (%d/%d)", j.Filename, partNum, totalParts)

		msgID, err := j.Client.PostArticle(ctx, subject, body)
		if err != nil {
			return fmt.Errorf("post part %d/%d: %w", partNum, totalParts, err)
		}
		segments = append(segments, nntp.NZBSegment{
			Bytes:  len(body),
			Number: partNum,
			MsgID:  msgID,
		})

		pct := int(float64(partNum) / float64(totalParts) * 95.0)
		j.Worker.Progress(ctx, j.JobID, pct)
	}

	nzb, err := nntp.BuildNZB(j.Filename, j.Newsgroup, j.Poster, totalParts, segments)
	if err != nil {
		return fmt.Errorf("build nzb: %w", err)
	}

	sum := sha256.Sum256(nzb)
	artifactHash := hex.EncodeToString(sum[:])

	if err := os.MkdirAll(j.NZBDir, 0o755); err != nil {
		return fmt.Errorf("nzb dir: %w", err)
	}
	nzbRelPath := artifactHash + ".nzb"
	nzbFullPath := filepath.Join(j.NZBDir, nzbRelPath)
	if err := os.WriteFile(nzbFullPath, nzb, 0o644); err != nil {
		return fmt.Errorf("write nzb: %w", err)
	}

	if err := j.Worker.queries.PromoteArtifactToReady(ctx, j.ArtifactID, artifactHash, nzbRelPath); err != nil {
		return fmt.Errorf("promote artifact: %w", err)
	}

	_ = os.Remove(j.TmpPath)

	j.Worker.Done(ctx, j.JobID, Event{ArtifactID: artifactHash})
	_ = j.Worker.queries.UpdateJobArtifactID(ctx, j.JobID, artifactHash)
	return nil
}

// StartFromPaymentConfirm looks up the artifact+job associated with a payment
// and kicks off the upload worker. Used by the NOWPayments webhook handler.
func StartFromPaymentConfirm(
	ctx context.Context,
	q *vdb.Queries,
	worker *Worker,
	client *nntp.Client,
	paymentID string,
	nzbDir, poster string,
) error {
	artifact, err := q.GetArtifactByPaymentID(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("lookup artifact: %w", err)
	}

	// Find the upload job attached to this artifact (the one we created at init time).
	row := q.DB().QueryRowContext(ctx, `
		SELECT id, tmp_path FROM jobs
		 WHERE artifact_id = ? AND type = 'upload'
		 ORDER BY created_at DESC LIMIT 1
	`, artifact.ID)
	var jobID string
	var tmpPath string
	if err := row.Scan(&jobID, &tmpPath); err != nil {
		return fmt.Errorf("lookup upload job: %w", err)
	}

	j := &UploadJob{
		JobID:      jobID,
		ArtifactID: artifact.ID,
		Filename:   artifact.Filename,
		TmpPath:    tmpPath,
		NZBDir:     nzbDir,
		Poster:     poster,
		Newsgroup:  client.Newsgroup(),
		Client:     client,
		Worker:     worker,
	}
	worker.Submit(ctx, j)
	return nil
}
