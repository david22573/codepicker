package agent

import (
	"testing"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func TestTrimMessageHistory(t *testing.T) {
	tests := []struct {
		name        string
		messages    []openrouter.ChatMessage
		maxRecent   int
		expectedLen int
	}{
		{
			name: "no trimming needed - under limit",
			messages: []openrouter.ChatMessage{
				{Role: "user", Content: "task"},
				{Role: "assistant", Content: "response 1"},
				{Role: "user", Content: "followup"},
			},
			maxRecent:   10,
			expectedLen: 3,
		},
		{
			name: "trimming needed - keeps first and recent",
			messages: []openrouter.ChatMessage{
				{Role: "user", Content: "original task"},   // index 0 - should keep
				{Role: "assistant", Content: "response 1"}, // index 1 - should drop
				{Role: "tool", Content: "tool result 1"},   // index 2 - should drop
				{Role: "assistant", Content: "response 2"}, // index 3 - should drop
				{Role: "tool", Content: "tool result 2"},   // index 4 - should drop
				{Role: "assistant", Content: "response 3"}, // index 5 - should keep (recent)
				{Role: "tool", Content: "tool result 3"},   // index 6 - should keep (recent)
				{Role: "assistant", Content: "response 4"}, // index 7 - should keep (recent)
			},
			maxRecent:   3,
			expectedLen: 4, // original task + 3 recent
		},
		{
			name: "exact boundary",
			messages: []openrouter.ChatMessage{
				{Role: "user", Content: "task"},
				{Role: "assistant", Content: "r1"},
				{Role: "assistant", Content: "r2"},
				{Role: "assistant", Content: "r3"},
				{Role: "assistant", Content: "r4"},
				{Role: "assistant", Content: "r5"},
			},
			maxRecent:   5,
			expectedLen: 6, // 1 original + 5 recent = 6 total
		},
		{
			name: "single message",
			messages: []openrouter.ChatMessage{
				{Role: "user", Content: "task"},
			},
			maxRecent:   10,
			expectedLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimMessageHistory(tt.messages, tt.maxRecent)

			if len(result) != tt.expectedLen {
				t.Errorf("Expected length %d, got %d", tt.expectedLen, len(result))
			}

			// Verify first message is preserved
			if len(tt.messages) > 0 && len(result) > 0 {
				if result[0].Role != tt.messages[0].Role || result[0].Content != tt.messages[0].Content {
					t.Errorf("First message not preserved correctly")
				}
			}

			// Verify last messages are preserved
			if len(result) > 1 {
				expectedLastIdx := len(tt.messages) - 1
				actualLastIdx := len(result) - 1

				if result[actualLastIdx].Role != tt.messages[expectedLastIdx].Role {
					t.Errorf("Last message not preserved correctly")
				}
			}
		})
	}
}

func TestTrimMessageHistoryPreservesOriginalTask(t *testing.T) {
	// Create a scenario with many messages
	messages := make([]openrouter.ChatMessage, 0, 50)
	messages = append(messages, openrouter.ChatMessage{
		Role:    "user",
		Content: "ORIGINAL_TASK_MARKER",
	})

	// Add many intermediate messages
	for i := 0; i < 40; i++ {
		messages = append(messages, openrouter.ChatMessage{
			Role:    "assistant",
			Content: "intermediate",
		})
	}

	// Add recent messages
	messages = append(messages, openrouter.ChatMessage{
		Role:    "assistant",
		Content: "RECENT_MESSAGE",
	})

	trimmed := trimMessageHistory(messages, 5)

	// Should have: original task + 5 recent = 6 messages
	if len(trimmed) != 6 {
		t.Fatalf("Expected 6 messages, got %d", len(trimmed))
	}

	// First message should be the original task
	if trimmed[0].Content != "ORIGINAL_TASK_MARKER" {
		t.Errorf("Original task not preserved as first message")
	}

	// Last message should be the recent one
	if trimmed[len(trimmed)-1].Content != "RECENT_MESSAGE" {
		t.Errorf("Recent message not preserved as last message")
	}
}

func TestMemoryLeakPrevention(t *testing.T) {
	// Simulate a long conversation that would cause memory leak
	messages := make([]openrouter.ChatMessage, 0, 100)
	messages = append(messages, openrouter.ChatMessage{
		Role:    "user",
		Content: "task",
	})

	// Simulate 99 additional messages (like in a long agent session)
	for i := 0; i < 99; i++ {
		messages = append(messages, openrouter.ChatMessage{
			Role:    "assistant",
			Content: "response",
		})
		messages = append(messages, openrouter.ChatMessage{
			Role:    "tool",
			Content: "tool output",
		})
	}

	// Initial size: 1 + 99*2 = 199 messages
	initialLen := len(messages)
	if initialLen != 199 {
		t.Fatalf("Test setup error: expected 199 messages, got %d", initialLen)
	}

	// Apply trimming with maxRecent=20
	trimmed := trimMessageHistory(messages, 20)

	// Should be: 1 (original) + 20 (recent) = 21 messages
	if len(trimmed) != 21 {
		t.Errorf("Expected 21 messages after trimming, got %d", len(trimmed))
	}

	// Verify memory is actually freed (in a real scenario)
	// This demonstrates the fix prevents unbounded growth
	reductionPercent := float64(len(trimmed)) / float64(initialLen) * 100
	t.Logf("Memory reduction: from %d to %d messages (%.1f%% of original)",
		initialLen, len(trimmed), reductionPercent)

	if reductionPercent > 15 {
		t.Errorf("Memory reduction not significant enough: %.1f%%", reductionPercent)
	}
}
