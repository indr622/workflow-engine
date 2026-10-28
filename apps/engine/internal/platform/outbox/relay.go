package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// Start menentukan posisi awal kursor consumer yang baru pertama kali terdaftar.
type Start uint8

const (
	// StartNow: hanya event dari transaksi yang belum selesai saat registrasi dan sesudahnya.
	StartNow Start = iota
	// StartBeginning: semua event yang masih ada di outbox (dalam retensi).
	StartBeginning
)

// Consumer adalah subscriber durable. Handle dipanggil di dalam transaksi relay yang memajukan
// kursor: efek DB handler commit bersama kursor (exactly-once untuk efek DB). Efek di luar DB wajib
// idempoten berdasarkan Event.ID karena batch bisa dikirim ulang.
type Consumer struct {
	// Name unik dan stabil lintas versi, mis. "timer.scheduler".
	Name string
	// Topics kosong berarti semua topik.
	Topics []string
	Handle func(ctx context.Context, tx pgx.Tx, ev Event) error
	// MaxAttempts sebelum event masuk dead letter; default 10.
	MaxAttempts int
	StartFrom   Start
}

// Options mengatur relay.
type Options struct {
	PollInterval time.Duration // default 200 ms
	BatchSize    int           // default 100
	// LockRetry adalah jeda mencoba ulang advisory lock consumer yang dipegang replika lain.
	LockRetry time.Duration // default 5 s
	// BackoffMin dan BackoffMax membatasi jeda setelah event gagal.
	BackoffMin, BackoffMax time.Duration // default 100 ms dan 30 s
}

func (o *Options) defaults() {
	if o.PollInterval <= 0 {
		o.PollInterval = 200 * time.Millisecond
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
	if o.LockRetry <= 0 {
		o.LockRetry = 5 * time.Second
	}
	if o.BackoffMin <= 0 {
		o.BackoffMin = 100 * time.Millisecond
	}
	if o.BackoffMax <= 0 {
		o.BackoffMax = 30 * time.Second
	}
}

// Relay mengirim event outbox ke consumer terdaftar.
type Relay struct {
	pool      *pgxpool.Pool
	log       *slog.Logger
	opts      Options
	mu        sync.Mutex
	consumers []*Consumer
	wake      []chan struct{}
}

// NewRelay membuat relay di atas pool.
func NewRelay(pool *pgxpool.Pool, log *slog.Logger, opts Options) *Relay {
	opts.defaults()
	if log == nil {
		log = slog.Default()
	}
	return &Relay{pool: pool, log: log.With(slog.String("component", "outbox")), opts: opts}
}

// Register mendaftarkan consumer sebelum Run atau Drain.
func (r *Relay) Register(c Consumer) error {
	if c.Name == "" || c.Handle == nil {
		return errors.New("outbox consumer needs a name and a handler")
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.consumers {
		if existing.Name == c.Name {
			return fmt.Errorf("outbox consumer %q registered twice", c.Name)
		}
	}
	r.consumers = append(r.consumers, &c)
	r.wake = append(r.wake, make(chan struct{}, 1))
	return nil
}

// Run memproses semua consumer sampai ctx dibatalkan: satu goroutine per consumer ditambah satu
// listener NOTIFY yang membangunkan semuanya.
func (r *Relay) Run(ctx context.Context) error {
	r.mu.Lock()
	consumers, wake := append([]*Consumer(nil), r.consumers...), append([]chan struct{}(nil), r.wake...)
	r.mu.Unlock()
	if len(consumers) == 0 {
		<-ctx.Done()
		return nil
	}
	for _, c := range consumers {
		if err := r.ensureCursor(ctx, c); err != nil {
			return err
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.listen(ctx, wake)
	}()
	for i, c := range consumers {
		wg.Add(1)
		go func(c *Consumer, wake <-chan struct{}) {
			defer wg.Done()
			r.runConsumer(ctx, c, wake)
		}(c, wake[i])
	}
	wg.Wait()
	return nil
}

// Drain memproses semua consumer sampai tidak ada event yang bisa dibaca lagi, tanpa advisory lock.
// Dipakai test dan perintah admin.
func (r *Relay) Drain(ctx context.Context) error {
	r.mu.Lock()
	consumers := append([]*Consumer(nil), r.consumers...)
	r.mu.Unlock()
	for _, c := range consumers {
		if err := r.ensureCursor(ctx, c); err != nil {
			return err
		}
		for {
			n, failed, err := r.processBatch(ctx, c, r.opts.BatchSize)
			if err != nil {
				return err
			}
			if failed {
				continue
			}
			if n == 0 {
				break
			}
		}
	}
	return nil
}

// listen meneruskan NOTIFY outbox_wakeup ke semua consumer; bila koneksi putus, mencoba lagi.
func (r *Relay) listen(ctx context.Context, wake []chan struct{}) {
	for ctx.Err() == nil {
		err := func() error {
			conn, err := r.pool.Acquire(ctx)
			if err != nil {
				return err
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, "SET statement_timeout = 0"); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "LISTEN "+WakeupChannel); err != nil {
				return err
			}
			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					return err
				}
				for _, ch := range wake {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}()
		if ctx.Err() != nil {
			return
		}
		r.log.Warn("outbox listener restarting", slog.Any("error", err))
		sleep(ctx, time.Second)
	}
}

// runConsumer memegang advisory lock consumer lalu memproses batch sampai ctx selesai.
func (r *Relay) runConsumer(ctx context.Context, c *Consumer, wake <-chan struct{}) {
	log := r.log.With(slog.String("consumer", c.Name))
	for ctx.Err() == nil {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			sleep(ctx, r.opts.LockRetry)
			continue
		}
		var locked bool
		err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('outbox:' || $1, 0))", c.Name).Scan(&locked)
		if err != nil || !locked {
			conn.Release()
			sleep(ctx, r.opts.LockRetry)
			continue
		}
		log.Info("outbox consumer acquired")
		r.serve(ctx, c, wake, conn, log)
		// Koneksi pemegang lock dilepas paksa agar lock sesi ikut terlepas.
		_ = conn.Conn().Close(context.Background())
		conn.Release()
		log.Info("outbox consumer released")
	}
}

func (r *Relay) serve(ctx context.Context, c *Consumer, wake <-chan struct{}, lockConn *pgxpool.Conn, log *slog.Logger) {
	backoff := time.Duration(0)
	for ctx.Err() == nil {
		if err := lockConn.Ping(ctx); err != nil {
			log.Warn("outbox consumer lost its lock connection", slog.Any("error", err))
			return
		}
		n, failed, err := r.processBatch(ctx, c, r.opts.BatchSize)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			log.Warn("outbox batch failed", slog.Any("error", err))
			backoff = nextBackoff(backoff, r.opts)
			sleep(ctx, backoff)
			continue
		case failed:
			backoff = nextBackoff(backoff, r.opts)
			sleep(ctx, backoff)
			continue
		}
		backoff = 0
		if n >= r.opts.BatchSize {
			continue
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-time.After(r.opts.PollInterval):
		}
	}
}

func (r *Relay) ensureCursor(ctx context.Context, c *Consumer) error {
	start := `INSERT INTO outbox_cursor (consumer, tx_id, event_id)
VALUES ($1, pg_snapshot_xmin(pg_current_snapshot()), '00000000-0000-0000-0000-000000000000')
ON CONFLICT (consumer) DO NOTHING`
	if c.StartFrom == StartBeginning {
		start = `INSERT INTO outbox_cursor (consumer, tx_id, event_id)
VALUES ($1, '0'::xid8, '00000000-0000-0000-0000-000000000000')
ON CONFLICT (consumer) DO NOTHING`
	}
	if _, err := r.pool.Exec(ctx, start, c.Name); err != nil {
		return fmt.Errorf("init cursor %s: %w", c.Name, err)
	}
	return nil
}

const selectBatchSQL = `SELECT e.event_id::text, e.tx_id::text, e.topic, e.aggregate_type, e.aggregate_key::text,
       e.aggregate_seq, e.payload, e.created_at
FROM outbox_event e
WHERE (e.tx_id, e.event_id) > ($1::xid8, $2::uuid)
  AND e.tx_id < pg_snapshot_xmin(pg_current_snapshot())
  AND (cardinality($3::text[]) = 0 OR e.topic = ANY($3::text[]))
ORDER BY e.tx_id, e.event_id
LIMIT $4`

type pending struct {
	ev   Event
	txID string
}

// processBatch mengirim paling banyak limit event dalam satu transaksi. Bila handler gagal di event
// ke-i, transaksi dibatalkan; event sebelum i dikirim ulang dalam batch berukuran i agar kemajuannya
// ter-commit, lalu kegagalan event ke-i dicatat (attempts++, atau dead letter). failed bernilai true
// bila ada event yang gagal (pemanggil memberi jeda backoff).
func (r *Relay) processBatch(ctx context.Context, c *Consumer, limit int) (int, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var curTx, curID string
	var attempts int
	err = tx.QueryRow(ctx, `SELECT tx_id::text, event_id::text, attempts FROM outbox_cursor WHERE consumer = $1 FOR UPDATE`, c.Name).
		Scan(&curTx, &curID, &attempts)
	if err != nil {
		return 0, false, fmt.Errorf("read cursor %s: %w", c.Name, err)
	}
	topics := c.Topics
	if topics == nil {
		topics = []string{}
	}
	rows, err := tx.Query(ctx, selectBatchSQL, curTx, curID, topics, limit)
	if err != nil {
		return 0, false, err
	}
	var batch []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.ev.ID, &p.txID, &p.ev.Topic, &p.ev.AggregateType, &p.ev.AggregateKey,
			&p.ev.AggregateSeq, &p.ev.Payload, &p.ev.OccurredAt); err != nil {
			rows.Close()
			return 0, false, err
		}
		batch = append(batch, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	if len(batch) == 0 {
		return 0, false, nil
	}
	for i, p := range batch {
		if herr := r.handle(ctx, c, tx, p.ev); herr != nil {
			_ = tx.Rollback(ctx)
			if i > 0 {
				// Commit kemajuan event sebelum i; event bermasalah kini di depan kursor, jadi
				// kegagalannya dicatat sebagai satu percobaan.
				n, _, err := r.processBatch(ctx, c, i)
				if err != nil {
					return n, true, err
				}
				return n, true, r.recordFailure(ctx, c, p, herr)
			}
			return 0, true, r.recordFailure(ctx, c, p, herr)
		}
	}
	last := batch[len(batch)-1]
	if _, err := tx.Exec(ctx, `UPDATE outbox_cursor SET tx_id = $2::xid8, event_id = $3::uuid, attempts = 0, last_error = NULL, updated_at = now() WHERE consumer = $1`,
		c.Name, last.txID, last.ev.ID); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	m := metrics.Get()
	for _, p := range batch {
		m.OutboxDelivered.Add(ctx, 1, metrics.Attr("consumer", c.Name, "topic", p.ev.Topic))
	}
	return len(batch), false, nil
}

// handle memanggil handler; panic diubah menjadi error agar relay tidak mati.
func (r *Relay) handle(ctx context.Context, c *Consumer, tx pgx.Tx, ev Event) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return c.Handle(ctx, tx, ev)
}

// recordFailure mencatat kegagalan event di depan kursor; setelah MaxAttempts, event dipindah ke
// dead letter dan kursor maju melewatinya, dalam satu transaksi.
func (r *Relay) recordFailure(ctx context.Context, c *Consumer, p pending, cause error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attempts int
	if err := tx.QueryRow(ctx, `UPDATE outbox_cursor SET attempts = attempts + 1, last_error = $2, updated_at = now()
WHERE consumer = $1 RETURNING attempts`, c.Name, cause.Error()).Scan(&attempts); err != nil {
		return err
	}
	log := r.log.With(slog.String("consumer", c.Name), slog.String("event_id", p.ev.ID), slog.Int("attempts", attempts))
	metrics.Get().OutboxFailed.Add(ctx, 1, metrics.Attr("consumer", c.Name, "topic", p.ev.Topic))
	if attempts < c.MaxAttempts {
		log.Warn("outbox event failed", slog.Any("error", cause))
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_dead_letter (consumer, event_id, topic, aggregate_key, payload, error, attempts)
VALUES ($1, $2::uuid, $3, $4::uuid, $5, $6, $7) ON CONFLICT (consumer, event_id) DO NOTHING`,
		c.Name, p.ev.ID, p.ev.Topic, p.ev.AggregateKey, []byte(p.ev.Payload), cause.Error(), attempts); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE outbox_cursor SET tx_id = $2::xid8, event_id = $3::uuid, attempts = 0, updated_at = now() WHERE consumer = $1`,
		c.Name, p.txID, p.ev.ID); err != nil {
		return err
	}
	log.Error("outbox event dead lettered", slog.Any("error", cause))
	metrics.Get().OutboxDeadLetter.Add(ctx, 1, metrics.Attr("consumer", c.Name, "topic", p.ev.Topic))
	return tx.Commit(ctx)
}

// Requeue memindahkan event dari dead letter kembali ke outbox (event baru dengan payload sama)
// agar dikirim ulang ke consumer tersebut dan consumer lain.
func Requeue(ctx context.Context, pool *pgxpool.Pool, w *Writer, consumer, eventID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ev Event
	err = tx.QueryRow(ctx, `DELETE FROM outbox_dead_letter WHERE consumer = $1 AND event_id = $2::uuid
RETURNING topic, aggregate_key::text, payload`, consumer, eventID).Scan(&ev.Topic, &ev.AggregateKey, &ev.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no dead letter %s for consumer %s", eventID, consumer)
	}
	if err != nil {
		return err
	}
	ev.AggregateType = "requeued"
	if err := w.Enqueue(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Prune menghapus event yang sudah dilewati semua kursor dan lebih tua dari retention.
func Prune(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM outbox_event e
WHERE e.created_at < now() - make_interval(secs => $1)
  AND NOT EXISTS (SELECT 1 FROM outbox_cursor c WHERE (c.tx_id, c.event_id) < (e.tx_id, e.event_id))`,
		retention.Seconds())
	if err != nil {
		return 0, fmt.Errorf("prune outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ConsumerStatus adalah lag satu consumer untuk perintah admin.
type ConsumerStatus struct {
	Consumer    string
	Pending     int64
	Attempts    int
	LastError   string
	DeadLetters int64
}

// Status mengembalikan lag semua consumer yang punya kursor.
func Status(ctx context.Context, pool *pgxpool.Pool) ([]ConsumerStatus, error) {
	rows, err := pool.Query(ctx, `SELECT c.consumer,
  (SELECT count(*) FROM outbox_event e WHERE (e.tx_id, e.event_id) > (c.tx_id, c.event_id)),
  c.attempts, COALESCE(c.last_error, ''),
  (SELECT count(*) FROM outbox_dead_letter d WHERE d.consumer = c.consumer)
FROM outbox_cursor c ORDER BY c.consumer`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ConsumerStatus, error) {
		var s ConsumerStatus
		err := row.Scan(&s.Consumer, &s.Pending, &s.Attempts, &s.LastError, &s.DeadLetters)
		return s, err
	})
}

func nextBackoff(cur time.Duration, o Options) time.Duration {
	next := cur * 2
	if next < o.BackoffMin {
		next = o.BackoffMin
	}
	if next > o.BackoffMax {
		next = o.BackoffMax
	}
	// Jitter ±20% agar replika tidak mencoba ulang serempak.
	jitter := time.Duration(rand.Int64N(int64(next)/5 + 1)) //nolint:gosec // jitter, bukan kriptografi
	return next - next/10 + jitter
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
