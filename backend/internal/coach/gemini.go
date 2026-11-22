package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/"

type GeminiClient struct {
	apiKey, model string
	httpClient    *http.Client
	endpoint      string
}

func NewGeminiClient(apiKey, model string, timeout time.Duration) *GeminiClient {
	return &GeminiClient{apiKey: apiKey, model: model, httpClient: &http.Client{Timeout: timeout}, endpoint: geminiEndpoint}
}
func (g *GeminiClient) Name() string { return "gemini" }

type geminiRequest struct {
	SystemInstruction struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"systemInstruction"`
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
	GenerationConfig struct {
		MaxOutputTokens int     `json:"maxOutputTokens"`
		Temperature     float64 `json:"temperature"`
	} `json:"generationConfig"`
}
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error struct {
		Code   int    `json:"code"`
		Status string `json:"status"`
	} `json:"error"`
}

func (g *GeminiClient) Generate(ctx context.Context, system string, messages []Message, maxTokens int) (ProviderResult, error) {
	if g.apiKey == "" || g.model == "" || g.httpClient == nil || len(messages) == 0 || maxTokens < 1 || maxTokens > 2048 {
		return ProviderResult{}, ErrUnavailable
	}
	var body geminiRequest
	body.SystemInstruction.Parts = append(body.SystemInstruction.Parts, struct {
		Text string `json:"text"`
	}{system})
	body.GenerationConfig.MaxOutputTokens = maxTokens
	body.GenerationConfig.Temperature = 0.2
	for _, message := range messages {
		role := message.Role
		if role == "assistant" {
			role = "model"
		}
		if role != "user" && role != "model" {
			return ProviderResult{}, ErrInvalid
		}
		entry := struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{Role: role}
		entry.Parts = append(entry.Parts, struct {
			Text string `json:"text"`
		}{message.Content})
		body.Contents = append(body.Contents, entry)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ProviderResult{}, err
	}
	endpoint := g.endpoint + url.PathEscape(g.model) + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ProviderResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ProviderResult{}, ErrTimeout
		}
		if ctx.Err() != nil {
			return ProviderResult{}, ctx.Err()
		}
		return ProviderResult{}, ErrUnavailable
	}
	defer resp.Body.Close()
	responseRaw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		return ProviderResult{}, ErrUnavailable
	}
	if len(responseRaw) > 1<<20 {
		return ProviderResult{}, ErrMalformedResponse
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusTooManyRequests {
			return ProviderResult{}, &ProviderFailure{Cause: ErrProviderQuota, Delay: parseRetryAfter(resp.Header.Get("Retry-After"))}
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
			return ProviderResult{}, ErrProviderConfig
		}
		return ProviderResult{}, ErrUnavailable
	}
	var decoded geminiResponse
	if err = json.Unmarshal(responseRaw, &decoded); err != nil {
		return ProviderResult{}, ErrMalformedResponse
	}
	if len(decoded.Candidates) == 0 {
		return ProviderResult{}, ErrMalformedResponse
	}
	parts := decoded.Candidates[0].Content.Parts
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" || len(text) > 12000 {
		return ProviderResult{}, ErrMalformedResponse
	}
	return ProviderResult{Text: text, Model: g.model, InputTokens: decoded.UsageMetadata.PromptTokenCount, OutputTokens: decoded.UsageMetadata.CandidatesTokenCount}, nil
}

func parseRetryAfter(raw string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && seconds > 0 && seconds <= 3600 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		d := time.Until(at)
		if d > 0 && d <= time.Hour {
			return d
		}
	}
	return 0
}
