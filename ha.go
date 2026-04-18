package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type HAClient struct {
	BaseURL    string
	authHeader string
	HTTPClient *http.Client
}

type HAEntityState struct {
	EntityID     string
	State        string
	FriendlyName string
}

func NewHAClient(baseURL, token string) *HAClient {
	return &HAClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Bearer " + token,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HAClient) GetState(ctx context.Context, entityID string) (string, error) {
	url := fmt.Sprintf("%s/api/states/%s", c.BaseURL, entityID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call HA API: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return "", err
	}

	var result struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain remainder for connection reuse
	return result.State, nil
}

type servicePayload struct {
	EntityID string `json:"entity_id"`
}

func (c *HAClient) CallService(ctx context.Context, domain, service, entityID string) error {
	url := fmt.Sprintf("%s/api/services/%s/%s", c.BaseURL, domain, service)

	body, err := json.Marshal(servicePayload{EntityID: entityID})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call HA API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if err := checkStatus(resp); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(respBody))
	}
	log.Printf("[HA] %s/%s: %s", domain, service, bytes.TrimSpace(respBody))
	return nil
}

func (c *HAClient) GetAllStates(ctx context.Context) ([]HAEntityState, error) {
	url := fmt.Sprintf("%s/api/states", c.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call HA API: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var raw []struct {
		EntityID   string `json:"entity_id"`
		State      string `json:"state"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
		} `json:"attributes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	states := make([]HAEntityState, len(raw))
	for i, r := range raw {
		states[i] = HAEntityState{
			EntityID:     r.EntityID,
			State:        r.State,
			FriendlyName: r.Attributes.FriendlyName,
		}
	}
	return states, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HA API returned status %d", resp.StatusCode)
	}
	return nil
}

func DomainService(entityID string) (domain, service string) {
	prefix, _, _ := strings.Cut(entityID, ".")
	switch prefix {
	case "lock":
		return "lock", "unlock"
	case "cover":
		return "cover", "open_cover"
	case "button":
		return "button", "press"
	case "script":
		return "script", "turn_on"
	default:
		return prefix, "open"
	}
}
