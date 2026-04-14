package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const geetestVerifyURL = "https://gcaptcha4.geetest.com/validate"

type geeTestVerifier struct {
	httpClient *http.Client
	verifyURL  string
}

func NewGeeTestVerifier() service.GeeTestVerifier {
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &geeTestVerifier{
		httpClient: sharedClient,
		verifyURL:  geetestVerifyURL,
	}
}

func (v *geeTestVerifier) VerifyToken(ctx context.Context, captchaID, captchaKey string, payload *service.GeeTestValidateRequest) (*service.GeeTestVerifyResponse, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	verifyURL, err := buildGeeTestVerifyURL(v.verifyURL, captchaID)
	if err != nil {
		return nil, fmt.Errorf("build verify url: %w", err)
	}

	formData := url.Values{}
	formData.Set("lot_number", payload.LotNumber)
	formData.Set("captcha_output", payload.CaptchaOutput)
	formData.Set("pass_token", payload.PassToken)
	formData.Set("gen_time", payload.GenTime)
	formData.Set("sign_token", buildGeeTestSignToken(payload.LotNumber, captchaKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result service.GeeTestVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

func buildGeeTestVerifyURL(baseURL, captchaID string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("captcha_id", captchaID)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildGeeTestSignToken(lotNumber, captchaKey string) string {
	mac := hmac.New(sha256.New, []byte(captchaKey))
	_, _ = mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}
