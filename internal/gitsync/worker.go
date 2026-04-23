package gitsync

import (
	"context"
	"math/rand"
	"time"

	"go.uber.org/zap"
)

// worker polls a single GitSource on a configurable interval.
type worker struct {
	gs      *GitSource
	svc     *Service
	stopCh  chan struct{}
}

func newWorker(gs *GitSource, svc *Service) *worker {
	return &worker{
		gs:     gs,
		svc:    svc,
		stopCh: make(chan struct{}),
	}
}

func (w *worker) stop() {
	close(w.stopCh)
}

// run is the goroutine body. It fires an immediate sync on start, then polls.
func (w *worker) run(ctx context.Context) {
	logger := w.svc.logger.With(zap.String("source", w.gs.Name), zap.String("id", w.gs.ID))
	logger.Info("poller started", zap.Int("interval_secs", w.gs.PollIntervalSeconds))

	// Sync immediately on start
	w.doSync(ctx, false)

	interval := time.Duration(w.gs.PollIntervalSeconds) * time.Second
	for {
		// Add ±10% jitter so multiple sources don't all fire at once
		jitter := time.Duration(rand.Int63n(int64(interval/10)*2) - int64(interval/10))
		timer := time.NewTimer(interval + jitter)

		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Info("poller stopped (context cancelled)")
			return
		case <-w.stopCh:
			timer.Stop()
			logger.Info("poller stopped")
			return
		case <-timer.C:
			w.doSync(ctx, false)
		}
	}
}

func (w *worker) doSync(ctx context.Context, forceAll bool) {
	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Re-read the source from store to pick up any config changes made via API
	gs, err := w.svc.store.Get(syncCtx, w.gs.ID)
	if err != nil {
		w.svc.logger.Error("poller: failed to reload source config",
			zap.String("id", w.gs.ID), zap.Error(err))
		return
	}
	w.gs = gs // update local copy

	if _, err := w.svc.runSync(syncCtx, gs, forceAll); err != nil {
		w.svc.logger.Error("poller: sync error",
			zap.String("source", gs.Name), zap.Error(err))
	}
}
