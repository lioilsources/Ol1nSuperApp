package jobs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lioilsources/ol1n-vault/internal/nntp"
)

type DownloadJob struct {
	JobID       string
	ArtifactID  string
	Filename    string
	ContentType string
	NZBPath     string // full path to the NZB on disk
	FilesDir    string
	TokenTTL    time.Duration
	PublicBase  string

	Client *nntp.Client
	Worker *Worker
}

func (j *DownloadJob) ID() string { return j.JobID }

func (j *DownloadJob) Run(ctx context.Context) error {
	raw, err := os.ReadFile(j.NZBPath)
	if err != nil {
		return fmt.Errorf("read nzb: %w", err)
	}
	parsed, err := nntp.ParseNZB(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if len(parsed.Files) == 0 || len(parsed.Files[0].Segments.Segment) == 0 {
		return fmt.Errorf("nzb: no segments")
	}
	segments := parsed.Files[0].Segments.Segment
	sort.Slice(segments, func(i, k int) bool { return segments[i].Number < segments[k].Number })

	// Assemble file body by fetching each segment in order.
	var body bytes.Buffer
	for idx, seg := range segments {
		articleBody, err := j.Client.FetchBody(ctx, seg.MsgID)
		if err != nil {
			return fmt.Errorf("fetch segment %d: %w", seg.Number, err)
		}
		dec, err := nntp.YEncDecode(bytes.NewReader(articleBody))
		if err != nil {
			return fmt.Errorf("decode segment %d: %w", seg.Number, err)
		}
		body.Write(dec.Data)
		pct := int(float64(idx+1) / float64(len(segments)) * 90.0)
		j.Worker.Progress(ctx, j.JobID, pct)
	}

	outDir := filepath.Join(j.FilesDir, j.ArtifactID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("files dir: %w", err)
	}
	outPath := filepath.Join(outDir, j.Filename)
	if err := os.WriteFile(outPath, body.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	token, err := newToken()
	if err != nil {
		return err
	}
	if err := j.Worker.queries.InsertDownloadToken(ctx, token, j.ArtifactID, j.TokenTTL); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/files/%s/%s", strings.TrimRight(j.PublicBase, "/"), token, j.Filename)

	j.Worker.Done(ctx, j.JobID, Event{
		DownloadURL: url,
		ContentType: j.ContentType,
		ArtifactID:  j.ArtifactID,
	})
	return nil
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
