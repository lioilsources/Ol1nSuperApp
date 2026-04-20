package jobs

import (
	"context"
	"log/slog"
	"sync"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/sse"
)

// Event is what we publish on the SSE broker for a given job.
type Event struct {
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Job interface {
	ID() string
	Run(ctx context.Context) error
}

type Worker struct {
	queries *vdb.Queries
	broker  *sse.Broker
	sem     chan struct{}
	wg      sync.WaitGroup
}

func NewWorker(q *vdb.Queries, broker *sse.Broker, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Worker{
		queries: q,
		broker:  broker,
		sem:     make(chan struct{}, concurrency),
	}
}

// Submit runs j on a worker goroutine, updating DB status and publishing SSE
// events as the job progresses. Non-blocking beyond acquiring a slot.
func (w *Worker) Submit(ctx context.Context, j Job) {
	w.sem <- struct{}{}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() { <-w.sem }()

		jobID := j.ID()
		_ = w.queries.UpdateJobStatus(ctx, jobID, vdb.JobRunning, 0)
		w.broker.Publish(jobID, Event{Status: vdb.JobRunning, Progress: 0})

		if err := j.Run(ctx); err != nil {
			slog.Error("job failed", "id", jobID, "err", err)
			_ = w.queries.MarkJobFailed(ctx, jobID, err.Error())
			w.broker.Publish(jobID, Event{Status: vdb.JobFailed, Error: err.Error()})
			return
		}
	}()
}

// Wait blocks until all submitted jobs have completed. Useful for graceful shutdown.
func (w *Worker) Wait() { w.wg.Wait() }

// Progress emits both a DB row update and an SSE event. Jobs call this.
func (w *Worker) Progress(ctx context.Context, jobID string, pct int) {
	_ = w.queries.UpdateJobProgress(ctx, jobID, pct)
	w.broker.Publish(jobID, Event{Status: vdb.JobRunning, Progress: pct})
}

// Done marks the job complete and pushes a terminal SSE event.
func (w *Worker) Done(ctx context.Context, jobID string, ev Event) {
	_ = w.queries.MarkJobDone(ctx, jobID, ev.DownloadURL)
	ev.Status = vdb.JobDone
	ev.Progress = 100
	w.broker.Publish(jobID, ev)
}
