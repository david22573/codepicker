package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
)

type ReActAgent struct {
	model      agent.LLMClient
	tools      map[string]agent.Tool
	bus        *event.DataBus // NEW: Event Bus integration
	logger     *logging.Logger
	controller *AdaptiveController
	processor  *ObservationProcessor
	sysMsg     string
}

func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	bus *event.DataBus,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	budget float64,
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	for _, t := range tools {
		toolMap[t.Name()] = t
	}

	return &ReActAgent{
		model:      model,
		tools:      toolMap,
		bus:        bus,
		logger:     logger,
		controller: NewAdaptiveController(10, 30, costTracker, budget),
		processor:  NewObservationProcessor(8000),
		sysMsg:     "You are CodePicker. Use 'Thought:', 'Actions: [...]', or 'Final Answer:'.",
	}
}

func (a *ReActAgent) Name() string { return "CodePicker-Event-v5" }

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	maxTurns := a.controller.CalculateAllowedTurns(0.5)
	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	for i := 0; i < maxTurns; i++ {
		// LLM Turn
		response, err := a.model.Chat(ctx, a.sysMsg, currentContext)
		if err != nil {
			a.bus.Publish(event.Event{Type: event.EventError, Payload: map[string]interface{}{"error": err.Error()}})
			return "", err
		}

		thought, actions := ParseBatchActions(response)

		// Emit Thought Event
		a.bus.Publish(event.Event{
			Type: event.EventAgentThought,
			Payload: map[string]interface{}{
				"turn":    i,
				"content": thought,
			},
			Timestamp: time.Now().Unix(),
		})

		if len(actions) == 0 && strings.Contains(response, "Final Answer:") {
			a.bus.Publish(event.Event{Type: event.EventAgentFinish, Payload: map[string]interface{}{"result": response}})
			return response, nil
		}

		var batchResults strings.Builder
		for _, act := range actions {
			// Emit Tool Start Event
			a.bus.Publish(event.Event{
				Type: event.EventToolStart,
				Payload: map[string]interface{}{
					"tool":  act.Tool,
					"input": string(act.Input),
				},
			})

			tool, _ := a.tools[act.Tool]
			out, err := tool.Execute(ctx, string(act.Input))

			// Emit Tool End Event
			status := "success"
			if err != nil {
				status = "error"
			}
			a.bus.Publish(event.Event{
				Type: event.EventToolEnd,
				Payload: map[string]interface{}{
					"tool":   act.Tool,
					"status": status,
					"output": out,
				},
			})

			batchResults.WriteString(fmt.Sprintf("\n[TOOL: %s]\n%s", act.Tool, out))
		}

		formattedObs := a.processor.FormatForKimi("batch", batchResults.String())
		currentContext += fmt.Sprintf("\nThought: %s\nObservation: %s\n", thought, formattedObs)
	}

	return "", fmt.Errorf("max turns exceeded")
}
