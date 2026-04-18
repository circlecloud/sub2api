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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// ForwardAsChatCompletionsViaChatUpstream 为 OpenAI API Key chat_upstream=chat_completions
// 的账号直连上游 /v1/chat/completions；其他账号回退到原有 Responses 中间态链路。
func (s *OpenAIGatewayService) ForwardAsChatCompletionsViaChatUpstream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	if account == nil || !account.UsesOpenAIChatCompletionsUpstream() {
		return s.ForwardAsChatCompletions(ctx, c, account, body, promptCacheKey, defaultMappedModel)
	}
	_ = promptCacheKey

	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	clientStream := gjson.GetBytes(body, "stream").Bool()
	ctx = s.EnsureOpenAIStreamFirstTokenRectifierContext(ctx, clientStream, 1, 1)
	serviceTier := extractOpenAIServiceTierFromBody(body)
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, originalModel)

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)

	upstreamBody := body
	if upstreamModel != "" && upstreamModel != originalModel {
		rewrittenBody, err := sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite chat completions model: %w", err)
		}
		upstreamBody = rewrittenBody
	}

	logger.L().Debug("openai chat_completions direct upstream: model mapping applied",
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
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return s.handleChatCompletionsChatUpstreamErrorResponse(ctx, resp, c, account)
	}

	var result *OpenAIForwardResult
	if clientStream {
		result, err = s.handleChatCompletionsChatUpstreamStreamingResponse(ctx, resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	} else {
		result, err = s.handleChatCompletionsChatUpstreamNonStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}
	if err != nil || result == nil {
		return result, err
	}
	result.ServiceTier = serviceTier
	result.ReasoningEffort = reasoningEffort
	return result, nil
}

func (s *OpenAIGatewayService) buildAPIKeyChatCompletionsUpstreamRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	validatedURL, err := s.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIChatCompletionsURL(validatedURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("authorization", "Bearer "+token)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if !openaiAllowedHeaders[lowerKey] {
				continue
			}
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	return req, nil
}

func (s *OpenAIGatewayService) handleChatCompletionsChatUpstreamErrorResponse(
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
	return s.handleChatCompletionsErrorResponse(resp, c, account)
}

func (s *OpenAIGatewayService) handleChatCompletionsChatUpstreamNonStreamingResponse(
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

	usage := OpenAIUsage{}
	if isEventStreamResponse(resp.Header) {
		finalResponse, parsedUsage, err := s.bufferChatCompletionsSSEToResponsesResponse(body, originalModel, requestID)
		if err != nil {
			writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream stream ended without a terminal response event")
			return nil, err
		}
		usage = parsedUsage
		chatResp := apicompat.ResponsesToChatCompletions(finalResponse, originalModel)
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		c.JSON(resp.StatusCode, chatResp)
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

	if parsedUsage, ok := extractOpenAIChatCompletionsUsageFromJSONBytes(body); ok {
		usage = parsedUsage
	}
	if originalModel != upstreamModel {
		body = s.replaceModelInResponseBody(body, upstreamModel, originalModel)
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
	c.Data(resp.StatusCode, contentType, body)

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

func (s *OpenAIGatewayService) handleChatCompletionsChatUpstreamStreamingResponse(
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

	usage := OpenAIUsage{}
	var firstTokenMs *int
	var streamFirstEventMs *int
	streamGateStart := time.Now()
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	upstreamRequestID := strings.TrimSpace(requestID)
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

	processLine := func(line string) {
		if data, ok := extractOpenAISSEDataLine(line); ok {
			trimmedData := strings.TrimSpace(data)
			if trimmedData == "[DONE]" {
				sawDone = true
			}
			if openAIChatCompletionsStreamEventIsTerminal([]byte(trimmedData)) {
				sawTerminalEvent = true
			}
			if firstTokenMs == nil && trimmedData != "" && trimmedData != "[DONE]" {
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
			parseOpenAIChatCompletionsSSEUsageBytes([]byte(trimmedData), &usage)
			if originalModel != upstreamModel {
				line = s.replaceModelInSSELine(line, upstreamModel, originalModel)
			}
		}

		if !clientDisconnected {
			if _, err := fmt.Fprintln(c.Writer, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI chat upstream] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
			} else {
				flusher.Flush()
			}
		}
	}

	handleScanErr := func(err error) (*OpenAIForwardResult, error) {
		if err == nil {
			return nil, nil
		}
		if sawTerminalEvent {
			return resultWithUsage(), nil
		}
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI chat upstream] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return resultWithUsage(), err
		}
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI chat upstream] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return resultWithUsage(), fmt.Errorf("stream read error: %w", err)
	}

	finalizeStream := func() (*OpenAIForwardResult, error) {
		if !clientDisconnected && !sawDone && !sawTerminalEvent && ctx.Err() == nil {
			logger.FromContext(ctx).With(
				zap.String("component", "service.openai_gateway"),
				zap.Int64("account_id", account.ID),
				zap.String("upstream_request_id", upstreamRequestID),
			).Info("OpenAI chat upstream 上游流在未收到 [DONE] 时结束，疑似断流")
			return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
		}
		return resultWithUsage(), nil
	}

	if !rectifierEnabled {
		for scanner.Scan() {
			processLine(scanner.Text())
		}
		if result, err := handleScanErr(scanner.Err()); result != nil || err != nil {
			return result, err
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
				return handleScanErr(ev.err)
			}
			processLine(ev.line)
		case <-firstEventCh:
			if firstTokenMs != nil {
				continue
			}
			_ = resp.Body.Close()
			return nil, s.newOpenAIStreamFirstTokenRectifierTimeoutError(ctx, c, account, rectifier, "first_token")
		}
	}
}

func extractOpenAIChatCompletionsUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, false
	}
	values := gjson.GetManyBytes(
		body,
		"usage.prompt_tokens",
		"usage.completion_tokens",
		"usage.prompt_tokens_details.cached_tokens",
	)
	if !values[0].Exists() && !values[1].Exists() && !values[2].Exists() {
		return OpenAIUsage{}, false
	}
	return OpenAIUsage{
		InputTokens:          int(values[0].Int()),
		OutputTokens:         int(values[1].Int()),
		CacheReadInputTokens: int(values[2].Int()),
	}, true
}

func parseOpenAIChatCompletionsSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) || !gjson.ValidBytes(data) {
		return
	}
	values := gjson.GetManyBytes(
		data,
		"usage.prompt_tokens",
		"usage.completion_tokens",
		"usage.prompt_tokens_details.cached_tokens",
	)
	if !values[0].Exists() && !values[1].Exists() && !values[2].Exists() {
		return
	}
	usage.InputTokens = int(values[0].Int())
	usage.OutputTokens = int(values[1].Int())
	usage.CacheReadInputTokens = int(values[2].Int())
}

func parseOpenAIChatCompletionsSSEUsageFromBody(body string) OpenAIUsage {
	usage := OpenAIUsage{}
	for _, line := range strings.Split(body, "\n") {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		parseOpenAIChatCompletionsSSEUsageBytes([]byte(data), &usage)
	}
	return usage
}

func openAIUsageFromChatCompletionsUsage(usage *apicompat.ChatUsage) OpenAIUsage {
	out := OpenAIUsage{}
	if usage == nil {
		return out
	}
	out.InputTokens = usage.PromptTokens
	out.OutputTokens = usage.CompletionTokens
	if usage.PromptTokensDetails != nil {
		out.CacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
	}
	return out
}

func openAIUsageFromResponsesUsage(usage *apicompat.ResponsesUsage) OpenAIUsage {
	out := OpenAIUsage{}
	if usage == nil {
		return out
	}
	out.InputTokens = usage.InputTokens
	out.OutputTokens = usage.OutputTokens
	if usage.InputTokensDetails != nil {
		out.CacheReadInputTokens = usage.InputTokensDetails.CachedTokens
	}
	return out
}

func (s *OpenAIGatewayService) bufferChatCompletionsSSEToResponsesResponse(body []byte, model string, requestID string) (*apicompat.ResponsesResponse, OpenAIUsage, error) {
	state := apicompat.NewChatCompletionsToResponsesState()
	state.Model = model
	acc := apicompat.NewBufferedResponseAccumulator()
	usage := OpenAIUsage{}
	var finalResponse *apicompat.ResponsesResponse

	processEvent := func(event apicompat.ResponsesStreamEvent) {
		acc.ProcessEvent(&event)
		if (event.Type == "response.completed" || event.Type == "response.incomplete" || event.Type == "response.failed") && event.Response != nil {
			finalResponse = event.Response
			usage = openAIUsageFromResponsesUsage(event.Response.Usage)
		}
	}

	for _, line := range strings.Split(string(body), "\n") {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			logger.L().Warn("openai chat upstream non-streaming fallback: failed to parse chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			continue
		}
		if chunk.Usage != nil {
			usage = openAIUsageFromChatCompletionsUsage(chunk.Usage)
		}
		for _, event := range apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state) {
			processEvent(event)
		}
	}

	if finalResponse == nil {
		for _, event := range apicompat.FinalizeChatCompletionsResponsesStream(state) {
			processEvent(event)
		}
	}
	if finalResponse == nil {
		return nil, usage, errors.New("upstream stream ended without terminal response event")
	}
	acc.SupplementResponseOutput(finalResponse)
	if finalResponse.Usage != nil {
		usage = openAIUsageFromResponsesUsage(finalResponse.Usage)
	}
	return finalResponse, usage, nil
}

func openAIChatCompletionsStreamEventIsTerminal(data []byte) bool {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return false
	}
	choices := gjson.GetBytes(data, "choices").Array()
	for _, choice := range choices {
		finishReason := choice.Get("finish_reason")
		if finishReason.Raw != "" && finishReason.Raw != "null" {
			return true
		}
	}
	return false
}
