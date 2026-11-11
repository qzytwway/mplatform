package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxErrorBodyBytes = 4096

type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	// IsEqual is nil when omitted, which Alertmanager reads as true.
	IsEqual *bool `json:"isEqual,omitempty"`
}

type SilenceStatus struct {
	State string `json:"state"`
}

type Silence struct {
	ID        string         `json:"id,omitempty"`
	Matchers  []Matcher      `json:"matchers"`
	StartsAt  time.Time      `json:"startsAt"`
	EndsAt    time.Time      `json:"endsAt"`
	CreatedBy string         `json:"createdBy"`
	Comment   string         `json:"comment"`
	Status    *SilenceStatus `json:"status,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty"`
}

// Client talks to the Alertmanager v2 API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(alertmanagerURL string, rt http.RoundTripper) (*Client, error) {
	parsed, err := url.Parse(alertmanagerURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid alertmanager URL %q", alertmanagerURL)
	}
	return &Client{
		baseURL:    strings.TrimRight(alertmanagerURL, "/"),
		httpClient: &http.Client{Transport: rt, Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) ListSilences(ctx context.Context) ([]Silence, error) {
	var silences []Silence
	if err := c.do(ctx, http.MethodGet, "/api/v2/silences", nil, &silences); err != nil {
		return nil, fmt.Errorf("failed to list silences: %w", err)
	}
	return silences, nil
}

func (c *Client) CreateSilence(ctx context.Context, silence Silence) (string, error) {
	var result struct {
		SilenceID string `json:"silenceID"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v2/silences", silence, &result); err != nil {
		return "", fmt.Errorf("failed to create silence: %w", err)
	}
	return result.SilenceID, nil
}

func (c *Client) ExpireSilence(ctx context.Context, id string) error {
	if err := c.do(ctx, http.MethodDelete, "/api/v2/silence/"+url.PathEscape(id), nil, nil); err != nil {
		return fmt.Errorf("failed to expire silence %s: %w", id, err)
	}
	return nil
}

type PostableAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
}

func (c *Client) PostAlerts(ctx context.Context, alerts []PostableAlert) error {
	if err := c.do(ctx, http.MethodPost, "/api/v2/alerts", alerts, nil); err != nil {
		return fmt.Errorf("failed to post alerts: %w", err)
	}
	return nil
}

// LoadedConfig returns the configuration Alertmanager runs with, its secrets
// masked.
func (c *Client) LoadedConfig(ctx context.Context) (string, error) {
	var status struct {
		Config struct {
			Original string `json:"original"`
		} `json:"config"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/status", nil, &status); err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}
	return status.Config.Original, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return fmt.Errorf("alertmanager returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
