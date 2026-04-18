package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsToResponsesResponse_BasicMapping(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:     "chatcmpl_123",
		Object: "chat.completion",
		Model:  "gpt-5-mini",
		Choices: []ChatChoice{{
			Index:        0,
			FinishReason: "tool_calls",
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`"I will check that."`),
				ReasoningContent: "Need to inspect tool output first.",
				ToolCalls: []ChatToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: ChatFunctionCall{
						Name:      "lookup_weather",
						Arguments: `{"city":"NYC"}`,
					},
				}},
			},
		}},
		Usage: &ChatUsage{
			PromptTokens:     11,
			CompletionTokens: 7,
			TotalTokens:      18,
			PromptTokensDetails: &ChatTokenDetails{
				CachedTokens: 3,
			},
		},
	}

	responsesResp := ChatCompletionsToResponsesResponse(resp)
	require.NotNil(t, responsesResp)

	assert.True(t, strings.HasPrefix(responsesResp.ID, "resp_"))
	assert.NotEqual(t, "chatcmpl_123", responsesResp.ID)
	assert.Equal(t, "response", responsesResp.Object)
	assert.Equal(t, "gpt-5-mini", responsesResp.Model)
	assert.Equal(t, "completed", responsesResp.Status)

	require.Len(t, responsesResp.Output, 3)
	assert.Equal(t, "reasoning", responsesResp.Output[0].Type)
	require.Len(t, responsesResp.Output[0].Summary, 1)
	assert.Equal(t, "Need to inspect tool output first.", responsesResp.Output[0].Summary[0].Text)

	assert.Equal(t, "message", responsesResp.Output[1].Type)
	assert.Equal(t, "assistant", responsesResp.Output[1].Role)
	require.Len(t, responsesResp.Output[1].Content, 1)
	assert.Equal(t, "output_text", responsesResp.Output[1].Content[0].Type)
	assert.Equal(t, "I will check that.", responsesResp.Output[1].Content[0].Text)

	assert.Equal(t, "function_call", responsesResp.Output[2].Type)
	assert.Equal(t, "call_1", responsesResp.Output[2].CallID)
	assert.Equal(t, "lookup_weather", responsesResp.Output[2].Name)
	assert.Equal(t, `{"city":"NYC"}`, responsesResp.Output[2].Arguments)

	require.NotNil(t, responsesResp.Usage)
	assert.Equal(t, 11, responsesResp.Usage.InputTokens)
	assert.Equal(t, 7, responsesResp.Usage.OutputTokens)
	assert.Equal(t, 18, responsesResp.Usage.TotalTokens)
	require.NotNil(t, responsesResp.Usage.InputTokensDetails)
	assert.Equal(t, 3, responsesResp.Usage.InputTokensDetails.CachedTokens)
}

func TestChatCompletionsChunkToResponsesEvents_TextDeltaAndCompletion(t *testing.T) {
	state := NewChatCompletionsToResponsesState()

	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_text",
		Model: "gpt-5",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{Role: "assistant"},
		}},
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "response.created", events[0].Type)
	require.NotNil(t, events[0].Response)
	assert.True(t, strings.HasPrefix(events[0].Response.ID, "resp_"))


	idx := 0
	events = ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_tool",
		Model: "gpt-5",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{ToolCalls: []ChatToolCall{{
				Index: &idx,
				ID:    "call_1",
				Type:  "function",
				Function: ChatFunctionCall{
					Name:      "lookup_weather",
					Arguments: `{"city":`,
				},
			}}},
		}},
	}, state)
	require.Len(t, events, 2)
	assert.Equal(t, "response.output_item.added", events[0].Type)
	assert.Equal(t, "function_call", events[0].Item.Type)
	assert.Equal(t, "call_1", events[0].Item.CallID)
	assert.Equal(t, "response.function_call_arguments.delta", events[1].Type)
	assert.Equal(t, `{"city":`, events[1].Delta)

	events = ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_tool",
		Model: "gpt-5",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{ToolCalls: []ChatToolCall{{
				Index: &idx,
				Function: ChatFunctionCall{
					Arguments: `"NYC"}`,
				},
			}}},
		}},
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "response.function_call_arguments.delta", events[0].Type)
	assert.Equal(t, `"NYC"}`, events[0].Delta)

	toolCalls := "tool_calls"
	events = ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_tool",
		Model: "gpt-5",
		Choices: []ChatChunkChoice{{
			Index:        0,
			FinishReason: &toolCalls,
		}},
	}, state)
	require.Len(t, events, 2)
	assert.Equal(t, "response.function_call_arguments.done", events[0].Type)
	assert.Equal(t, "response.output_item.done", events[1].Type)

	events = ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_tool",
		Model: "gpt-5",
		Usage: &ChatUsage{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17},
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "response.completed", events[0].Type)
	require.NotNil(t, events[0].Response)
	assert.Equal(t, "completed", events[0].Response.Status)
	require.NotNil(t, events[0].Response.Usage)
	assert.Equal(t, 12, events[0].Response.Usage.InputTokens)
	assert.Equal(t, 5, events[0].Response.Usage.OutputTokens)
}
