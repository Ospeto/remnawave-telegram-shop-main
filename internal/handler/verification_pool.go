package handler

import (
	"context"
	"sync"
)

// Screenshot verification is the slowest thing the bot does: one receipt photo
// can hold a worker for up to a minute while the vision provider answers.
//
// P8: every photo message used to run that work inside the Telegram worker that
// delivered the update, so a handful of customers sending screenshots at the
// same time consumed every worker and stopped the bot from answering anything
// else — including the menu buttons those same customers were tapping.
//
// The verification now runs on a small dedicated pool. The update handler only
// enqueues the work, so a photo costs one short handler call instead of a worker
// for the whole OCR round trip.
const (
	// verificationPoolWorkers bounds how many screenshot verifications run at
	// once. Vision calls are network-bound, so this is about provider
	// concurrency, not CPU.
	verificationPoolWorkers = 4

	// verificationQueueSize bounds how many photos may wait. Anything beyond it
	// is refused with a localized retry message rather than queued forever: a
	// stale photo would verify against a purchase the customer has moved on from.
	verificationQueueSize = 32
)

type verificationJob struct {
	ctx context.Context
	run func(ctx context.Context)
}

type verificationPool struct {
	jobs chan verificationJob
}

func newVerificationPool(workers int, queueSize int) *verificationPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}

	pool := &verificationPool{jobs: make(chan verificationJob, queueSize)}
	for i := 0; i < workers; i++ {
		go pool.work()
	}
	return pool
}

func (p *verificationPool) work() {
	for job := range p.jobs {
		job.run(job.ctx)
	}
}

// submit hands work to the pool. It reports false when the queue is full, so the
// caller can tell the customer to retry instead of dropping the photo silently.
//
// The job context deliberately ignores cancellation of the update context: the
// Telegram update is finished the moment the handler returns, but the
// verification must still complete.
func (p *verificationPool) submit(ctx context.Context, run func(context.Context)) bool {
	if p == nil || run == nil {
		return false
	}

	select {
	case p.jobs <- verificationJob{ctx: context.WithoutCancel(ctx), run: run}:
		return true
	default:
		return false
	}
}

var (
	verificationPoolOnce   sync.Once
	verificationPoolShared *verificationPool
)

// screenshotVerificationPool returns the process-wide verification pool.
//
// It is a package-level singleton because Handler is passed around by value, so
// the pool cannot live on the struct without copying a channel per call site.
func screenshotVerificationPool() *verificationPool {
	verificationPoolOnce.Do(func() {
		verificationPoolShared = newVerificationPool(verificationPoolWorkers, verificationQueueSize)
	})
	return verificationPoolShared
}
