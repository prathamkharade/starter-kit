// Package jobs is a durable background queue on top of Postgres
// (SELECT … FOR UPDATE SKIP LOCKED). No Redis/RabbitMQ needed; scales to
// many workers. Enqueue inside the same transaction as your write to get
// exactly-once "write + side effect" semantics.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/platform/db"
)

type Handler func(ctx context.Context, payload json.RawMessage) error

type EnqueueOpt func(*enqueueOpts)

type enqueueOpts struct {
	runAt       time.Time
	maxAttempts int
}

func At(t time.Time) EnqueueOpt    { return func(o *enqueueOpts) { o.runAt = t } }
func MaxAttempts(n int) EnqueueOpt { return func(o *enqueueOpts) { o.maxAttempts = n } }

// Enqueue adds a job. q may be the pool or a pgx.Tx.
func Enqueue(ctx context.Context, q db.Querier, kind string, payload any, opts ...EnqueueOpt) error {
	o := enqueueOpts{runAt: time.Now(), maxAttempts: 5}
	for _, fn := range opts {
		fn(&o)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO jobs(kind, payload, run_at, max_attempts) VALUES ($1,$2,$3,$4)`,
		kind, b, o.runAt, o.maxAttempts)
	return err
}

// Permanent marks an error as non-retryable.
type Permanent struct{ Err error }

func (p Permanent) Error() string { return p.Err.Error() }

type periodic struct {
	name  string
	every time.Duration
	fn    func(context.Context) error
}

type Worker struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	handlers    map[string]Handler
	periodic    []periodic
	concurrency int
	poll        time.Duration
}

func NewWorker(pool *pgxpool.Pool, log *slog.Logger, concurrency int) *Worker {
	return &Worker{pool: pool, log: log, handlers: map[string]Handler{}, concurrency: max(1, concurrency), poll: 2 * time.Second}
}

func (w *Worker) Handle(kind string, h Handler) { w.handlers[kind] = h }

// Every runs fn periodically (in every worker process; make fn idempotent).
func (w *Worker) Every(d time.Duration, name string, fn func(context.Context) error) {
	w.periodic = append(w.periodic, periodic{name, d, fn})
}

func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.loop(ctx) }()
	}
	for _, p := range w.periodic {
		wg.Add(1)
		go func(p periodic) {
			defer wg.Done()
			t := time.NewTicker(p.every)
			defer t.Stop()
			for {
				if err := p.fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
					w.log.Error("periodic task failed", "task", p.name, "err", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}(p)
	}
	w.log.Info("worker started", "concurrency", w.concurrency, "kinds", len(w.handlers))
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	for ctx.Err() == nil {
		worked, err := w.runOne(ctx)
		if err != nil && ctx.Err() == nil {
			w.log.Error("job loop", "err", err)
		}
		if !worked {
			select {
			case <-ctx.Done():
			case <-time.After(w.poll):
			}
		}
	}
}

func (w *Worker) runOne(ctx context.Context) (bool, error) {
	var (
		id                    int64
		kind                  string
		payload               json.RawMessage
		attempts, maxAttempts int
	)
	err := w.pool.QueryRow(ctx, `
		UPDATE jobs SET status='running', locked_at=now(), attempts=attempts+1, updated_at=now()
		WHERE id = (SELECT id FROM jobs WHERE status='queued' AND run_at <= now()
		            ORDER BY run_at, id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, payload, attempts, max_attempts`).Scan(&id, &kind, &payload, &attempts, &maxAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	h, ok := w.handlers[kind]
	var jobErr error
	if !ok {
		jobErr = Permanent{fmt.Errorf("no handler for kind %q", kind)}
	} else {
		jctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		jobErr = safeRun(jctx, h, payload)
		cancel()
	}

	if jobErr == nil {
		// payload may hold personal data (email address, OTP) — drop it once done (data minimisation)
		_, err = w.pool.Exec(ctx, `UPDATE jobs SET status='done', payload='{}', updated_at=now(), last_error=NULL WHERE id=$1`, id)
		return true, err
	}
	var perm Permanent
	if errors.As(jobErr, &perm) || attempts >= maxAttempts {
		w.log.Error("job failed permanently", "id", id, "kind", kind, "err", jobErr)
		_, err = w.pool.Exec(ctx, `UPDATE jobs SET status='failed', last_error=$2, updated_at=now() WHERE id=$1`, id, jobErr.Error())
		return true, err
	}
	backoff := min(time.Duration(1<<attempts)*10*time.Second, time.Hour)
	w.log.Warn("job failed, retrying", "id", id, "kind", kind, "attempt", attempts, "in", backoff, "err", jobErr)
	_, err = w.pool.Exec(ctx, `UPDATE jobs SET status='queued', run_at=now()+$2::interval, last_error=$3, updated_at=now() WHERE id=$1`,
		id, fmt.Sprintf("%d seconds", int(backoff.Seconds())), jobErr.Error())
	return true, err
}

func safeRun(ctx context.Context, h Handler, p json.RawMessage) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, p)
}

// Maintenance requeues stuck jobs and deletes old finished ones.
func Maintenance(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		if _, err := pool.Exec(ctx, `UPDATE jobs SET status='queued', run_at=now()
			WHERE status='running' AND locked_at < now() - interval '45 minutes'`); err != nil {
			return err
		}
		_, err := pool.Exec(ctx, `DELETE FROM jobs WHERE status IN ('done','failed') AND updated_at < now() - interval '7 days'`)
		return err
	}
}
