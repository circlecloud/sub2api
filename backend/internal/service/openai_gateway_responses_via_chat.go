package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ForwardAsResponsesViaChatUpstream lets /v1/responses clients work against an
// API-key OpenAI account whose upstream protocol is configured as
// /v1/chat/completions.
func (s *OpenAIGatewayService) ForwardAsResponsesViaChatUpstream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if account == nil || !account.UsesOpenAIChatCompletionsUpstream() {
		return s.Forward(ctx, c, account, body)
	}

	startTime := time.Now()

	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := strings.TrimSpace(responsesReq.Model)
	clientStream := responsesReq.Stream
	ctx = s.EnsureOpenAIStreamFirstTokenRectifierContext(ctx, clientStream, 1, 1)
	serviceTier := extractOpenAIServiceTierFromBody(body)
	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body)

	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&responsesReq)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}
	if chatReq.Stream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	chatReq.Model = upstreamModel

	upstreamBody, err := json.Marshal(chatReq)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to serialize upstream request")
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}

	logger.L().Debug("openai responses via chat upstream: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	setOpsUpstreamRequestBody(c, upstreamBody)

	upstreamCtx := s.ApplyOpenAIStreamResponseHeaderRectifierContext(ctx, clientStream)
	if rectifier, ok := getOpenAIStreamFirstTokenRectifier(ctx); ok && clientStream {
		s.logOpenAIStreamResponseHeaderRectifierEnabled(ctx, account, rectifier)
	}
	upstreamReq, err := s.buildAPIKeyChatCompletionsUpstreamRequest(upstreamCtx, c, account, upstreamBody, token)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	prepareLatencyMs := time.Since(startTime).Milliseconds()
	SetOpsLatencyMs(c, OpsGatewayPrepareLatencyMsKey, prepareLatencyMs)
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		if _, ok := AsUpstreamResponseHeaderTimeoutError(err); ok {
			if rectifier, rectifierEnabled := getOpenAIStreamFirstTokenRectifier(ctx); rectifierEnabled {
				return nil, s.newOpenAIStreamFirstTokenRectifierTimeoutError(ctx, c, account, rectifier, "response_header")
			}
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeResponsesError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return s.handleResponsesViaChatUpstreamErrorResponse(ctx, resp, c, account)
	}

	var result *OpenAIForwardResult
	if clientStream {
		result, err = s.handleResponsesViaChatUpstreamStreamingResponse(ctx, resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	} else {
		result, err = s.handleResponsesViaChatUpstreamNonStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}
	if err != nil || result == nil {
		return result, err
	}
	result.ServiceTier = serviceTier
	result.ReasoningEffort = reasoningEffort
	return result, nil
}

func (s *OpenAIGatewayService) handleResponsesViaChatUpstreamErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*OpenAIForwardResult, error) {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(respBody), maxBytes)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
		}
	}
	return s.handleCompatErrorResponse(resp, c, account, writeResponsesError)
}

func (s *OpenAIGatewayService) handleResponsesViaChatUpstreamNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if isEventStreamResponse(resp.Header) {
		finalResponse, usage, err := s.bufferChatCompletionsSSEToResponsesResponse(body, originalModel, requestID)
		if err != nil {
			writeResponsesError(c, http.StatusBadGateway, "upstream_error", "Upstream stream ended without a terminal response event")
			return nil, err
		}
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		c.JSON(resp.StatusCode, finalResponse)
		return &OpenAIForwardResult{
			RequestID:     requestID,
			Usage:         usage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        false,
			Duration:      time.Since(startTime),
		}, nil
	}

	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		writeResponsesError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
		return nil, fmt.Errorf("parse chat completions response: %w", err)
	}

	responsesResp := apicompat.ChatCompletionsToResponsesResponse(&chatResp)
	responsesResp.Model = originalModel
	encoded, err := json.Marshal(responsesResp)
	if err != nil {
		writeResponsesError(c, http.StatusBadGateway, "upstream_error", "Failed to serialize response")
		return nil, fmt.Errorf("marshal responses response: %w", err)
	}

	usage := OpenAIUsage{}
	if chatResp.Usage != nil {
		usage.InputTokens = chatResp.Usage.PromptTokens
		usage.OutputTokens = chatResp.Usage.CompletionTokens
		if chatResp.Usage.PromptTokensDetails != nil {
			usage.CacheReadInputTokens = chatResp.Usage.PromptTokensDetails.CachedTokens
		}
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(resp.StatusCode, contentType, encoded)

	return &OpenAIForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) handleResponsesViaChatUpstreamStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	state := apicompat.NewChatCompletionsToResponsesState()
	state.Model = originalModel

	usage := OpenAIUsage{}
	var firstTokenMs *int
	var streamFirstEventMs *int
	sawDone := false
	sawTerminalSignal := false
	clientDisconnected := false
	streamGateStart := time.Now()
	rectifier, rectifierEnabled := getOpenAIStreamFirstTokenRectifier(ctx)
	var firstEventTimer *time.Timer
	var firstEventCh <-chan time.Time
	stopFirstEventGate := func() {
		if firstEventTimer == nil {
			firstEventCh = nil
			return
		}
		if !firstEventTimer.Stop() {
			select {
			case <-firstEventTimer.C:
			default:
			}
		}
		firstEventTimer = nil
		firstEventCh = nil
	}
	if rectifierEnabled {
		firstEventTimer = time.NewTimer(rectifier.FirstTokenTimeout)
		firstEventCh = firstEventTimer.C
		defer stopFirstEventGate()
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:          requestID,
			Usage:              usage,
			Model:              originalModel,
			BillingModel:       billingModel,
			UpstreamModel:      upstreamModel,
			Stream:             true,
			Duration:           time.Since(startTime),
			FirstTokenMs:       firstTokenMs,
			StreamFirstEventMs: streamFirstEventMs,
		}
	}

	writeSSE := func(sse string) {
		if clientDisconnected {
			return
		}
		if _, err := fmt.Fprint(c.Writer, sse); err != nil {
			clientDisconnected = true
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI responses via chat upstream] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
		}
	}

	processPayload := func(payload string) {
		if openAIChatCompletionsStreamEventIsTerminal([]byte(payload)) {
			sawTerminalSignal = true
		}
		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			logger.L().Warn("openai responses via chat upstream stream: failed to parse chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
			streamMs := int(time.Since(streamGateStart).Milliseconds())
			streamFirstEventMs = &streamMs
			SetOpsLatencyMs(c, OpsStreamFirstEventLatencyMsKey, int64(streamMs))
			stopFirstEventGate()
			if rectifierEnabled {
				s.logOpenAIStreamFirstTokenRectifierObserved(ctx, account, rectifier, ms, streamMs)
			}
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
			if chunk.Usage.PromptTokensDetails != nil {
				usage.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			}
		}

		events := apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state)
		for _, event := range events {
			if event.Type == "response.completed" || event.Type == "response.incomplete" || event.Type == "response.failed" {
				sawTerminalSignal = true
			}
			sse, err := apicompat.ResponsesEventToSSE(event)
			if err != nil {
				logger.L().Warn("openai responses via chat upstream stream: failed to marshal event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			writeSSE(sse)
		}
		if len(events) > 0 && !clientDisconnected {
			flusher.Flush()
		}
	}

	finalizeStream := func() (*OpenAIForwardResult, error) {
		if !sawDone && !sawTerminalSignal && ctx.Err() == nil {
			logger.L().Info("openai responses via chat upstream stream ended without terminal signal",
				zap.Int64("account_id", account.ID),
				zap.String("request_id", requestID),
			)
			return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
		}
		if finalEvents := apicompat.FinalizeChatCompletionsResponsesStream(state); len(finalEvents) > 0 {
			for _, event := range finalEvents {
				sse, err := apicompat.ResponsesEventToSSE(event)
				if err != nil {
					continue
				}
				writeSSE(sse)
			}
			if !clientDisconnected {
				flusher.Flush()
			}
		}
		return resultWithUsage(), nil
	}

	if !rectifierEnabled {
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				sawDone = true
				return finalizeStream()
			}
			processPayload(payload)
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai responses via chat upstream stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
		return finalizeStream()
	}

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	go func() {
		defer close(events)
		for scanner.Scan() {
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}()
	defer close(done)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return finalizeStream()
			}
			if ev.err != nil {
				if !errors.Is(ev.err, context.Canceled) && !errors.Is(ev.err, context.DeadlineExceeded) {
					logger.L().Warn("openai responses via chat upstream stream: read error",
						zap.Error(ev.err),
						zap.String("request_id", requestID),
					)
				}
				return finalizeStream()
			}
			line := ev.line
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				sawDone = true
				return finalizeStream()
			}
			processPayload(payload)
		case <-firstEventCh:
			if firstTokenMs != nil {
				continue
			}
			_ = resp.Body.Close()
			return nil, s.newOpenAIStreamFirstTokenRectifierTimeoutError(ctx, c, account, rectifier, "first_token")
		}
	}
}
