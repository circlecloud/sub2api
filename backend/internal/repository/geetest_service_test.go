package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GeeTestServiceSuite struct {
	suite.Suite
	ctx      context.Context
	verifier *geeTestVerifier
	received chan geeTestRequestCapture
}

type geeTestRequestCapture struct {
	Query url.Values
	Form  url.Values
}

func (s *GeeTestServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.received = make(chan geeTestRequestCapture, 1)
	verifier, ok := NewGeeTestVerifier().(*geeTestVerifier)
	require.True(s.T(), ok, "type assertion failed")
	s.verifier = verifier
}

func (s *GeeTestServiceSuite) setupTransport(handler http.HandlerFunc) {
	s.verifier.verifyURL = "http://in-process/geetest/validate"
	s.verifier.httpClient = &http.Client{
		Transport: newInProcessTransport(handler, nil),
	}
}

func TestGeeTestServiceSuite(t *testing.T) {
	suite.Run(t, new(GeeTestServiceSuite))
}

func TestBuildGeeTestSignToken(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("captcha-key"))
	_, _ = mac.Write([]byte("lot-123"))
	expected := hex.EncodeToString(mac.Sum(nil))
	require.Equal(t, expected, buildGeeTestSignToken("lot-123", "captcha-key"))
}

func (s *GeeTestServiceSuite) TestVerifyToken_SendsFormAndDecodesJSON() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		s.received <- geeTestRequestCapture{Query: r.URL.Query(), Form: values}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.GeeTestVerifyResponse{Result: "success"})
	}))

	resp, err := s.verifier.VerifyToken(s.ctx, "captcha-id", "captcha-key", &service.GeeTestValidateRequest{
		LotNumber:     "lot-123",
		CaptchaOutput: "output",
		PassToken:     "pass",
		GenTime:       "1710000000",
	})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), resp)
	require.Equal(s.T(), "success", resp.Result)

	select {
	case capture := <-s.received:
		require.Equal(s.T(), "captcha-id", capture.Query.Get("captcha_id"))
		require.Equal(s.T(), "lot-123", capture.Form.Get("lot_number"))
		require.Equal(s.T(), "output", capture.Form.Get("captcha_output"))
		require.Equal(s.T(), "pass", capture.Form.Get("pass_token"))
		require.Equal(s.T(), "1710000000", capture.Form.Get("gen_time"))

		mac := hmac.New(sha256.New, []byte("captcha-key"))
		_, _ = mac.Write([]byte("lot-123"))
		require.Equal(s.T(), hex.EncodeToString(mac.Sum(nil)), capture.Form.Get("sign_token"))
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *GeeTestServiceSuite) TestVerifyToken_ContentType() {
	var contentType string
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.GeeTestVerifyResponse{Result: "success"})
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "captcha-id", "captcha-key", &service.GeeTestValidateRequest{
		LotNumber:     "lot-123",
		CaptchaOutput: "output",
		PassToken:     "pass",
		GenTime:       "1710000000",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), strings.HasPrefix(contentType, "application/x-www-form-urlencoded"), "unexpected content-type: %s", contentType)
}

func (s *GeeTestServiceSuite) TestVerifyToken_RequestError() {
	s.verifier.verifyURL = "http://in-process/geetest/validate"
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	}

	_, err := s.verifier.VerifyToken(s.ctx, "captcha-id", "captcha-key", &service.GeeTestValidateRequest{
		LotNumber:     "lot-123",
		CaptchaOutput: "output",
		PassToken:     "pass",
		GenTime:       "1710000000",
	})
	require.Error(s.T(), err)
}

func (s *GeeTestServiceSuite) TestVerifyToken_InvalidJSON() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-valid-json")
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "captcha-id", "captcha-key", &service.GeeTestValidateRequest{
		LotNumber:     "lot-123",
		CaptchaOutput: "output",
		PassToken:     "pass",
		GenTime:       "1710000000",
	})
	require.Error(s.T(), err)
}

func (s *GeeTestServiceSuite) TestVerifyToken_UnexpectedStatusCode() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"status":"error"}`)
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "captcha-id", "captcha-key", &service.GeeTestValidateRequest{
		LotNumber:     "lot-123",
		CaptchaOutput: "output",
		PassToken:     "pass",
		GenTime:       "1710000000",
	})
	require.Error(s.T(), err)
}
