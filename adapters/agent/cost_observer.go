package agent

import (
	"context"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/logging"
	"go.uber.org/zap"
)

// CostObserver listens to the event bus and persists session metrics to the database.
type CostObserver struct {
	repo   domainAgent.Repository
	logger *logging.Logger
}

func NewCostObserver(repo domainAgent.Repository, logger *logging.Logger) *CostObserver {
	return &CostObserver{
		repo:   repo,
		logger: logger,
	}
}

// Start begins listening for cost events on the provided channel until the context is cancelled.
func (o *CostObserver) Start(ctx context.Context, ch <-chan event.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				if e.Type == event.EventSessionCost {
					o.handleCostUpdate(ctx, e)
				}
			}
		}
	}()
}

func (o *CostObserver) handleCostUpdate(ctx context.Context, e event.Event) {
	sessionID, ok := e.Payload["session_id"].(string)
	if !ok || sessionID == "" {
		return // Cannot track without an ID
	}

	totalTokens, _ := e.Payload["total_tokens"].(int)
	
	// Type assertion fallback for float64s depending on JSON unmarshalling behaviors
	totalCost := extractFloat(e.Payload["total_cost"])
	llmCost := extractFloat(e.Payload["llm_cost"])
	toolCost := extractFloat(e.Payload["tool_cost"])

	// Retrieve the existing execution record
	exec, err := o.repo.GetExecution(ctx, sessionID)
	if err != nil {
		// If it doesn't exist, this might be a stateless run or we missed the init.
		// We create a phantom execution record to ensure costs are tracked globally.
		exec = domainAgent.NewExecution(sessionID, "unknown-plan")
	}

	exec.RecordMetrics(totalCost, llmCost, toolCost, totalTokens)

	if err := o.repo.SaveExecution(ctx, exec); err != nil {
		o.logger.Error("failed to persist session cost metrics", zap.Error(err), zap.String("session_id", sessionID))
	} else {
		o.logger.Debug("persisted session cost metrics", zap.String("session_id", sessionID), zap.Float64("cost", totalCost))
	}
}

func extractFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	default:
		return 0.0
	}
}