package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	shared "ledgermeadow/src/shared/types"
)

type Syncer interface {
	Synchronize(ctx context.Context, connectionID shared.BankConnectionID) error
}

type Materializer interface {
	Materialize(ctx context.Context, userID shared.UserID) error
}

type Analyzer interface {
	Analyze(ctx context.Context, userID shared.UserID) error
}

type Worker struct {
	repository   *Repository
	syncer       Syncer
	analyzer     Analyzer
	materializer Materializer
	logger       *slog.Logger
	pollInterval time.Duration
}

func NewWorker(
	repository *Repository,
	syncer Syncer,
	analyzer Analyzer,
	materializer Materializer,
	logger *slog.Logger,
	pollInterval time.Duration,
) *Worker {
	return &Worker{
		repository: repository, syncer: syncer, analyzer: analyzer, materializer: materializer,
		logger: logger, pollInterval: pollInterval,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOne(ctx); err != nil && !errors.Is(err, ErrNoEvent) && !errors.Is(err, context.Canceled) {
			w.logger.Error("outbox processing failed", "operation", "outbox_process")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	event, err := w.repository.Claim(ctx)
	if err != nil {
		return err
	}
	switch event.EventType {
	case "BANK_CONNECTION_SYNC_REQUESTED":
		return w.processBankSync(ctx, event)
	case "TRANSACTION_ANALYSIS_REQUESTED":
		return w.processTransactionAnalysis(ctx, event)
	case "FINANCIAL_MATERIALIZATION_REQUESTED":
		return w.processMaterialization(ctx, event)
	default:
		return w.repository.Fail(ctx, event, "UNSUPPORTED_EVENT")
	}
}

func (w *Worker) processTransactionAnalysis(ctx context.Context, event Event) error {
	jobContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := w.analyzer.Analyze(jobContext, event.UserID); err != nil {
		w.logger.Warn("transaction analysis failed", "operation", "transaction_analysis")
		return w.repository.Fail(context.WithoutCancel(ctx), event, "TRANSACTION_ANALYSIS_FAILED")
	}
	w.logger.Info("transaction analysis completed", "operation", "transaction_analysis")
	return w.repository.CompleteAnalysis(ctx, event)
}

func (w *Worker) processBankSync(ctx context.Context, event Event) error {
	jobContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	connectionID := shared.BankConnectionID(event.AggregateID)
	if err := w.syncer.Synchronize(jobContext, connectionID); err != nil {
		w.logger.Warn("bank synchronization failed",
			"operation", "bank_sync", "connection_id", string(connectionID),
		)
		return w.repository.Fail(context.WithoutCancel(ctx), event, "SYNC_FAILED")
	}
	w.logger.Info("bank synchronization completed",
		"operation", "bank_sync", "connection_id", string(connectionID),
	)
	return w.repository.CompleteBankSync(ctx, event)
}

func (w *Worker) processMaterialization(ctx context.Context, event Event) error {
	jobContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := w.materializer.Materialize(jobContext, event.UserID); err != nil {
		w.logger.Warn("financial materialization failed", "operation", "financial_materialization")
		return w.repository.Fail(context.WithoutCancel(ctx), event, "MATERIALIZATION_FAILED")
	}
	w.logger.Info("financial materialization completed", "operation", "financial_materialization")
	return w.repository.CompleteMaterialization(ctx, event)
}
