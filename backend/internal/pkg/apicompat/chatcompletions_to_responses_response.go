package apicompat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ChatCompletionsToResponsesResponse converts a Chat Completions response into a
// Responses API response.
func ChatCompletionsToResponsesResponse(resp *ChatCompletionsResponse) *ResponsesResponse {
	if resp == nil {
		return nil
	}

	id := normalizeChatResponsesResponseID(resp.ID)
	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  resp.Model,
		Status: "completed",
	}

	var choice *ChatChoice
	if len(resp.Choices) > 0 {
		choice = &resp.Choices[0]
	}

	if choice != nil {
		status, incomplete := chatFinishReasonToResponsesStatus(choice.FinishReason)
		out.Status = status
		out.IncompleteDetails = incomplete
		out.Output = buildResponsesOutputsFromChatMessage(choice.Message)
	}
	if len(out.Output) == 0 {
		out.Output = []ResponsesOutput{{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "output_text", Text: ""}},
			Status:  "completed",
		}}
	}
	if resp.Usage != nil {
		out.Usage = chatUsageToResponsesUsage(resp.Usage)
	}

	return out
}

func normalizeChatResponsesResponseID(raw string) string {
	id := strings.TrimSpace(raw)
	if strings.HasPrefix(id, "resp_") {
		return id
	}
	return generateResponsesID()
}

func buildResponsesOutputsFromChatMessage(message ChatMessage) []ResponsesOutput {
	outputs := make([]ResponsesOutput, 0, 2+len(message.ToolCalls))
	if strings.TrimSpace(message.ReasoningContent) != "" {
		outputs = append(outputs, ResponsesOutput{
			Type: "reasoning",
			ID:   generateItemID(),
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: message.ReasoningContent,
			}},
		})
	}

	if content := extractChatResponseText(message.Content); content != "" || (len(message.ToolCalls) == 0 && message.FunctionCall == nil && len(outputs) == 0) {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "output_text", Text: content}},
			Status:  "completed",
		})
	}

	for _, toolCall := range message.ToolCalls {
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
			Status:    "completed",
		})
	}
	if message.FunctionCall != nil {
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    message.Name,
			Name:      message.FunctionCall.Name,
			Arguments: message.FunctionCall.Arguments,
			Status:    "completed",
		})
	}

	return outputs
}

func extractChatResponseText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	content, err := parseChatContent(raw)
	if err != nil {
		return ""
	}
	return content
}

func chatUsageToResponsesUsage(usage *ChatUsage) *ResponsesUsage {
	if usage == nil {
		return nil
	}
	out := &ResponsesUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		out.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: usage.PromptTokensDetails.CachedTokens,
		}
	}
	return out
}

func chatFinishReasonToResponsesStatus(finishReason string) (string, *ResponsesIncompleteDetails) {
	switch strings.TrimSpace(finishReason) {
	case "length":
		return "incomplete", &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	case "content_filter":
		return "incomplete", &ResponsesIncompleteDetails{Reason: "content_filter"}
	default:
		return "completed", nil
	}
}

type chatToolStreamState struct {
	ToolIndex   int
	OutputIndex int
	ItemID      string
	CallID      string
	Name        string
	Open        bool
}

// ChatCompletionsToResponsesState tracks streaming conversion state for
// translating Chat Completions SSE chunks into Responses SSE events.
type ChatCompletionsToResponsesState struct {
	ResponseID string
	Model      string
	Created    int64

	SequenceNumber int
	CreatedSent    bool
	CompletedSent  bool

	NextOutputIndex int

	ReasoningOpen        bool
	ReasoningItemID      string
	ReasoningOutputIndex int

	MessageOpen        bool
	MessageItemID      string
	MessageOutputIndex int

	ToolStates map[int]*chatToolStreamState

	PendingStatus            string
	PendingIncompleteDetails *ResponsesIncompleteDetails
	PendingUsage             *ResponsesUsage
}

// NewChatCompletionsToResponsesState returns an initialized stream state.
func NewChatCompletionsToResponsesState() *ChatCompletionsToResponsesState {
	return &ChatCompletionsToResponsesState{
		Created:    time.Now().Unix(),
		ToolStates: make(map[int]*chatToolStreamState),
	}
}

// ChatCompletionsChunkToResponsesEvents converts one Chat Completions chunk into
// zero or more Responses SSE events.
func ChatCompletionsChunkToResponsesEvents(
	chunk *ChatCompletionsChunk,
	state *ChatCompletionsToResponsesState,
) []ResponsesStreamEvent {
	if chunk == nil || state == nil {
		return nil
	}
	if state.ResponseID == "" {
		state.ResponseID = normalizeChatResponsesResponseID(chunk.ID)
	}
	if state.Model == "" {
		state.Model = strings.TrimSpace(chunk.Model)
	}

	var events []ResponsesStreamEvent
	if len(chunk.Choices) > 0 && !state.CreatedSent {
		events = append(events, makeChatResponsesCreatedEvent(state))
		state.CreatedSent = true
	}
	if chunk.Usage != nil {
		state.PendingUsage = chatUsageToResponsesUsage(chunk.Usage)
	}

	for _, choice := range chunk.Choices {
		events = append(events, chatChunkChoiceToResponsesEvents(choice, state)...)
	}

	if state.PendingStatus != "" && !state.CompletedSent && len(chunk.Choices) == 0 && state.PendingUsage != nil {
		events = append(events, makeChatResponsesCompletedEvent(state, state.PendingStatus, state.PendingIncompleteDetails, state.PendingUsage))
		state.CompletedSent = true
	}

	return events
}

func chatChunkChoiceToResponsesEvents(choice ChatChunkChoice, state *ChatCompletionsToResponsesState) []ResponsesStreamEvent {
	var events []ResponsesStreamEvent

	if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
		events = append(events, closeChatResponsesMessageItem(state)...)
		events = append(events, closeAllChatResponsesToolItems(state)...)
		if !state.ReasoningOpen {
			state.ReasoningOpen = true
			state.ReasoningItemID = generateItemID()
			state.ReasoningOutputIndex = state.NextOutputIndex
			state.NextOutputIndex++
			events = append(events, makeChatResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
				OutputIndex: state.ReasoningOutputIndex,
				Item: &ResponsesOutput{
					Type: "reasoning",
					ID:   state.ReasoningItemID,
				},
			}))
		}
		events = append(events, makeChatResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.ReasoningOutputIndex,
			SummaryIndex: 0,
			Delta:        *choice.Delta.ReasoningContent,
			ItemID:       state.ReasoningItemID,
		}))
	}

	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		events = append(events, closeChatResponsesReasoningItem(state)...)
		events = append(events, closeAllChatResponsesToolItems(state)...)
		if !state.MessageOpen {
			state.MessageOpen = true
			state.MessageItemID = generateItemID()
			state.MessageOutputIndex = state.NextOutputIndex
			state.NextOutputIndex++
			events = append(events, makeChatResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
				OutputIndex: state.MessageOutputIndex,
				Item: &ResponsesOutput{
					Type:   "message",
					ID:     state.MessageItemID,
					Role:   "assistant",
					Status: "in_progress",
				},
			}))
		}
		events = append(events, makeChatResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.MessageOutputIndex,
			ContentIndex: 0,
			Delta:        *choice.Delta.Content,
			ItemID:       state.MessageItemID,
		}))
	}

	if len(choice.Delta.ToolCalls) > 0 {
		events = append(events, closeChatResponsesReasoningItem(state)...)
		events = append(events, closeChatResponsesMessageItem(state)...)
		for idx, toolCall := range choice.Delta.ToolCalls {
			toolIndex := idx
			if toolCall.Index != nil {
				toolIndex = *toolCall.Index
			}
			toolState, toolAdded := ensureChatResponsesToolState(state, toolIndex, toolCall)
			if toolAdded {
				events = append(events, makeChatResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
					OutputIndex: toolState.OutputIndex,
					Item: &ResponsesOutput{
						Type:   "function_call",
						ID:     toolState.ItemID,
						CallID: toolState.CallID,
						Name:   toolState.Name,
						Status: "in_progress",
					},
				}))
			}
			if toolCall.Function.Arguments != "" {
				events = append(events, makeChatResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
					OutputIndex: toolState.OutputIndex,
					ItemID:      toolState.ItemID,
					CallID:      toolState.CallID,
					Name:        toolState.Name,
					Delta:       toolCall.Function.Arguments,
				}))
			}
		}
	}

	if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
		events = append(events, closeChatResponsesReasoningItem(state)...)
		events = append(events, closeChatResponsesMessageItem(state)...)
		events = append(events, closeAllChatResponsesToolItems(state)...)
		state.PendingStatus, state.PendingIncompleteDetails = chatFinishReasonToResponsesStatus(*choice.FinishReason)
		if state.PendingUsage != nil && !state.CompletedSent {
			events = append(events, makeChatResponsesCompletedEvent(state, state.PendingStatus, state.PendingIncompleteDetails, state.PendingUsage))
			state.CompletedSent = true
		}
	}

	return events
}

func ensureChatResponsesToolState(state *ChatCompletionsToResponsesState, toolIndex int, toolCall ChatToolCall) (*chatToolStreamState, bool) {
	if state.ToolStates == nil {
		state.ToolStates = make(map[int]*chatToolStreamState)
	}
	if existing, ok := state.ToolStates[toolIndex]; ok {
		if existing.CallID == "" && toolCall.ID != "" {
			existing.CallID = toolCall.ID
		}
		if existing.Name == "" && toolCall.Function.Name != "" {
			existing.Name = toolCall.Function.Name
		}
		return existing, false
	}

	callID := strings.TrimSpace(toolCall.ID)
	if callID == "" {
		callID = fmt.Sprintf("call_%d", toolIndex)
	}
	toolState := &chatToolStreamState{
		ToolIndex:   toolIndex,
		OutputIndex: state.NextOutputIndex,
		ItemID:      generateItemID(),
		CallID:      callID,
		Name:        toolCall.Function.Name,
		Open:        true,
	}
	state.NextOutputIndex++
	state.ToolStates[toolIndex] = toolState
	return toolState, true
}

// FinalizeChatCompletionsResponsesStream emits a synthetic terminal Responses
// event when the upstream chat stream ended without an explicit usage-bearing
// completion chunk.
func FinalizeChatCompletionsResponsesStream(state *ChatCompletionsToResponsesState) []ResponsesStreamEvent {
	if state == nil || !state.CreatedSent || state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent
	events = append(events, closeChatResponsesReasoningItem(state)...)
	events = append(events, closeChatResponsesMessageItem(state)...)
	events = append(events, closeAllChatResponsesToolItems(state)...)

	status := state.PendingStatus
	if status == "" {
		status = "completed"
	}
	events = append(events, makeChatResponsesCompletedEvent(state, status, state.PendingIncompleteDetails, state.PendingUsage))
	state.CompletedSent = true
	return events
}

func closeChatResponsesReasoningItem(state *ChatCompletionsToResponsesState) []ResponsesStreamEvent {
	if state == nil || !state.ReasoningOpen {
		return nil
	}
	outputIndex := state.ReasoningOutputIndex
	itemID := state.ReasoningItemID
	state.ReasoningOpen = false
	state.ReasoningItemID = ""
	state.ReasoningOutputIndex = 0
	return []ResponsesStreamEvent{
		makeChatResponsesEvent(state, "response.reasoning_summary_text.done", &ResponsesStreamEvent{
			OutputIndex:  outputIndex,
			SummaryIndex: 0,
			ItemID:       itemID,
		}),
		makeChatResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: outputIndex,
			Item: &ResponsesOutput{
				Type:   "reasoning",
				ID:     itemID,
				Status: "completed",
			},
		}),
	}
}

func closeChatResponsesMessageItem(state *ChatCompletionsToResponsesState) []ResponsesStreamEvent {
	if state == nil || !state.MessageOpen {
		return nil
	}
	outputIndex := state.MessageOutputIndex
	itemID := state.MessageItemID
	state.MessageOpen = false
	state.MessageItemID = ""
	state.MessageOutputIndex = 0
	return []ResponsesStreamEvent{
		makeChatResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
			OutputIndex:  outputIndex,
			ContentIndex: 0,
			ItemID:       itemID,
		}),
		makeChatResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: outputIndex,
			Item: &ResponsesOutput{
				Type:   "message",
				ID:     itemID,
				Role:   "assistant",
				Status: "completed",
			},
		}),
	}
}

func closeAllChatResponsesToolItems(state *ChatCompletionsToResponsesState) []ResponsesStreamEvent {
	if state == nil || len(state.ToolStates) == 0 {
		return nil
	}
	toolStates := make([]*chatToolStreamState, 0, len(state.ToolStates))
	for _, toolState := range state.ToolStates {
		if toolState != nil && toolState.Open {
			toolStates = append(toolStates, toolState)
		}
	}
	sort.Slice(toolStates, func(i, j int) bool {
		return toolStates[i].OutputIndex < toolStates[j].OutputIndex
	})

	events := make([]ResponsesStreamEvent, 0, len(toolStates)*2)
	for _, toolState := range toolStates {
		events = append(events,
			makeChatResponsesEvent(state, "response.function_call_arguments.done", &ResponsesStreamEvent{
				OutputIndex: toolState.OutputIndex,
				ItemID:      toolState.ItemID,
				CallID:      toolState.CallID,
				Name:        toolState.Name,
			}),
			makeChatResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
				OutputIndex: toolState.OutputIndex,
				Item: &ResponsesOutput{
					Type:   "function_call",
					ID:     toolState.ItemID,
					CallID: toolState.CallID,
					Name:   toolState.Name,
					Status: "completed",
				},
			}),
		)
		delete(state.ToolStates, toolState.ToolIndex)
	}
	return events
}

func makeChatResponsesCreatedEvent(state *ChatCompletionsToResponsesState) ResponsesStreamEvent {
	return makeChatResponsesEvent(state, "response.created", &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{},
		},
	})
}

func makeChatResponsesCompletedEvent(
	state *ChatCompletionsToResponsesState,
	status string,
	incompleteDetails *ResponsesIncompleteDetails,
	usage *ResponsesUsage,
) ResponsesStreamEvent {
	return makeChatResponsesEvent(state, responseTerminalEventType(status), &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            []ResponsesOutput{},
			Usage:             usage,
			IncompleteDetails: incompleteDetails,
		},
	})
}

func responseTerminalEventType(status string) string {
	if status == "incomplete" {
		return "response.incomplete"
	}
	return "response.completed"
}

func makeChatResponsesEvent(state *ChatCompletionsToResponsesState, eventType string, template *ResponsesStreamEvent) ResponsesStreamEvent {
	event := ResponsesStreamEvent{}
	if template != nil {
		event = *template
	}
	event.Type = eventType
	event.SequenceNumber = state.SequenceNumber
	state.SequenceNumber++
	return event
}
