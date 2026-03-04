package agent

import (
	"context"
	"sync"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/logging"
	"go.uber.org/zap"
)

type CostObserver struct {
	repo   domainAgent.Repository
	logger *logging.Logger
	writeQ chan event.Event
	wg     sync.WaitGroup
	stop   chan struct{}
}

func NewCostObserver(repo domainAgent.Repository, logger *logging.Logger) *CostObserver {
	return &CostObserver{
		repo:   repo,
		logger: logger,
		writeQ: make(chan event.Event, 1000), // Increased buffer to handle spikes
		stop:   make(chan struct{}),
	}
}

func (o *CostObserver) Start(ctx context.Context, ch <-chan event.Event) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

		batch := make(map[string]*domainAgent.Execution)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		flush := func() {
			for sessionID, exec := range batch {
				if err := o.repo.SaveExecution(context.Background(), exec); err != nil {
					o.logger.Error("failed to persist session cost metrics", zap.Error(err), zap.String("session_id", sessionID))
				} else {
					o.logger.Debug("persisted session cost metrics", zap.String("session_id", sessionID), zap.Float64("cost", exec.Cost))
				}
			}
			batch = make(map[string]*domainAgent.Execution)
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case <-o.stop:
				flush()
				return
			case <-ticker.C:
				flush()
			case e := <-o.writeQ:
				sessionID, ok := e.Payload["session_id"].(string)
				if !ok || sessionID == "" {
					continue
				}

				totalTokens, _ := e.Payload["total_tokens"].(int)
				totalCost := extractFloat(e.Payload["total_cost"])
				llmCost := extractFloat(e.Payload["llm_cost"])
				toolCost := extractFloat(e.Payload["tool_cost"])

				exec, exists := batch[sessionID]
				if !exists {
					var err error
					exec, err = o.repo.GetExecution(context.Background(), sessionID)
					if err != nil {
						exec = domainAgent.NewExecution(sessionID, "unknown-plan")
					}
					batch[sessionID] = exec
				}
				exec.RecordMetrics(totalCost, llmCost, toolCost, totalTokens)
			}
		}
	}()

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-o.stop:
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				if e.Type == event.EventSessionCost {
					select {
					case o.writeQ <- e:
					default:
						o.logger.Warn("Cost observer write queue full, dropping cost event", zap.String("session_id", e.Payload["session_id"].(string)))
					}
				}
			}
		}
	}()
}

func (o *CostObserver) Stop() {
	close(o.stop)
	o.wg.Wait()
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
