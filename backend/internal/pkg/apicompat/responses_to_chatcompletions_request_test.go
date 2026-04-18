package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToChatCompletionsRequest_BasicMapping(t *testing.T) {
	maxOutputTokens := 256
	strict := true
	temperature := 0.2
	topP := 0.9

	req := &ResponsesRequest{
		Model:        "gpt-5",
		Instructions: "You are helpful.",
		Input: json.RawMessage(`[
			{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,abc"}]},
			{"type":"function_call","call_id":"call_1","name":"lookup_weather","arguments":"{\"city\":\"NYC\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"sunny"}
		]`),
		MaxOutputTokens: &maxOutputTokens,
		Temperature:     &temperature,
		TopP:            &topP,
		Stream:          true,
		Tools: []ResponsesTool{{
			Type:        "function",
			Name:        "lookup_weather",
			Description: "Lookup the weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			Strict:      &strict,
		}},
		ToolChoice:  json.RawMessage(`{"type":"function","function":{"name":"lookup_weather"}}`),
		Reasoning:   &ResponsesReasoning{Effort: "high"},
		ServiceTier: "flex",
	}

	chatReq, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	assert.Equal(t, "gpt-5", chatReq.Model)
	assert.Empty(t, chatReq.Instructions)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	assert.Equal(t, 256, *chatReq.MaxCompletionTokens)
	assert.Nil(t, chatReq.MaxTokens)
	require.NotNil(t, chatReq.Temperature)
	assert.Equal(t, 0.2, *chatReq.Temperature)
	require.NotNil(t, chatReq.TopP)
	assert.Equal(t, 0.9, *chatReq.TopP)
	assert.True(t, chatReq.Stream)
	assert.Equal(t, "high", chatReq.ReasoningEffort)
	assert.Equal(t, "flex", chatReq.ServiceTier)
	assert.JSONEq(t, string(req.ToolChoice), string(chatReq.ToolChoice))

	require.Len(t, chatReq.Tools, 1)
	assert.Equal(t, "function", chatReq.Tools[0].Type)
	require.NotNil(t, chatReq.Tools[0].Function)
	assert.Equal(t, "lookup_weather", chatReq.Tools[0].Function.Name)
	assert.Equal(t, "Lookup the weather", chatReq.Tools[0].Function.Description)
	assert.JSONEq(t, `{"type":"object","properties":{"city":{"type":"string"}}}`, string(chatReq.Tools[0].Function.Parameters))
	require.NotNil(t, chatReq.Tools[0].Function.Strict)
	assert.True(t, *chatReq.Tools[0].Function.Strict)

	require.Len(t, chatReq.Messages, 4)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	var systemContent string
	require.NoError(t, json.Unmarshal(chatReq.Messages[0].Content, &systemContent))
	assert.Equal(t, "You are helpful.", systemContent)

	assert.Equal(t, "user", chatReq.Messages[1].Role)
	var userParts []ChatContentPart
	require.NoError(t, json.Unmarshal(chatReq.Messages[1].Content, &userParts))
	require.Len(t, userParts, 2)
	assert.Equal(t, "text", userParts[0].Type)
	assert.Equal(t, "hello", userParts[0].Text)
	require.NotNil(t, userParts[1].ImageURL)
	assert.Equal(t, "image_url", userParts[1].Type)
	assert.Equal(t, "data:image/png;base64,abc", userParts[1].ImageURL.URL)

	assert.Equal(t, "assistant", chatReq.Messages[2].Role)
	require.Len(t, chatReq.Messages[2].ToolCalls, 1)
	assert.Equal(t, "call_1", chatReq.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "function", chatReq.Messages[2].ToolCalls[0].Type)
	assert.Equal(t, "lookup_weather", chatReq.Messages[2].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"NYC"}`, chatReq.Messages[2].ToolCalls[0].Function.Arguments)

	assert.Equal(t, "tool", chatReq.Messages[3].Role)
	assert.Equal(t, "call_1", chatReq.Messages[3].ToolCallID)
	var toolContent string
	require.NoError(t, json.Unmarshal(chatReq.Messages[3].Content, &toolContent))
	assert.Equal(t, "sunny", toolContent)
}

func TestResponsesRequestNativeResponsesReasonsFromBody(t *testing.T) {
	reasons := ResponsesRequestNativeResponsesReasonsFromBody([]byte(`{"model":"gpt-5","previous_response_id":"resp_123","store":true,"include":["reasoning.encrypted_content"],"reasoning":{"summary":"detailed"},"tools":[{"type":"web_search"}],"input":[{"type":"computer_call"}]}`))
	require.Equal(t, []string{"previous_response_id", "store", "include", "reasoning.summary", "tools", "input"}, reasons)
	require.Equal(t,
		"chat_completions upstream compatibility mode only supports stateless /v1/responses requests; unsupported features: previous_response_id, store, include, reasoning.summary, tools, input",
		ResponsesRequestChatUpstreamUnsupportedMessage(reasons),
	)
}

func TestResponsesToChatCompletionsRequest_RejectsUnsupportedFields(t *testing.T) {
	baseReq := ResponsesRequest{
		Model: "gpt-5",
		Input: json.RawMessage(`[{"role":"user","content":"hello"}]`),
	}

	t.Run("previous_response_id", func(t *testing.T) {
		req := baseReq
		req.PreviousResponseID = "resp_123"

		_, err := ResponsesToChatCompletionsRequest(&req)
		require.Error(t, err)
		assert.ErrorContains(t, err, "stateless /v1/responses")
		assert.ErrorContains(t, err, "previous_response_id")
	})

	t.Run("store_true", func(t *testing.T) {
		req := baseReq
		store := true
		req.Store = &store

		_, err := ResponsesToChatCompletionsRequest(&req)
		require.Error(t, err)
		assert.ErrorContains(t, err, "stateless /v1/responses")
		assert.ErrorContains(t, err, "store")
	})

	t.Run("non_function_tool", func(t *testing.T) {
		req := baseReq
		req.Tools = []ResponsesTool{{Type: "web_search"}}

		_, err := ResponsesToChatCompletionsRequest(&req)
		require.Error(t, err)
		assert.ErrorContains(t, err, "stateless /v1/responses")
		assert.ErrorContains(t, err, "tools")
	})

	t.Run("aggregates_multiple_native_responses_features", func(t *testing.T) {
		req := baseReq
		store := true
		req.PreviousResponseID = "resp_123"
		req.Store = &store
		req.Include = []string{"reasoning.encrypted_content"}
		req.Tools = []ResponsesTool{{Type: "web_search"}}

		_, err := ResponsesToChatCompletionsRequest(&req)
		require.Error(t, err)
		assert.ErrorContains(t, err, "stateless /v1/responses")
		assert.ErrorContains(t, err, "previous_response_id")
		assert.ErrorContains(t, err, "store")
		assert.ErrorContains(t, err, "include")
		assert.ErrorContains(t, err, "tools")
	})
}
