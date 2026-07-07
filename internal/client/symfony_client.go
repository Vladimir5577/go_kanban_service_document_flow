package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/model"
)

type SymfonyClientInterface interface {
	FetchUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error)
}

type SymfonyClient struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

func NewSymfonyClient(cfg *config.Config) *SymfonyClient {
	return &SymfonyClient{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL: strings.TrimRight(cfg.SymfonyInternalApiUrl, "/"),
		apiKey:  cfg.SymfonyInternalApiKey,
	}
}

func (c *SymfonyClient) FetchUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if c.baseURL == "" || c.apiKey == "" {
		return nil, fmt.Errorf("symfony API credentials not configured")
	}

	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = strconv.FormatInt(id, 10)
	}
	joinedIDs := strings.Join(strIDs, ",")

	url := fmt.Sprintf("%s/api/internal/kanban/users?ids=%s", c.baseURL, joinedIDs)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("symfony request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("symfony API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var users []model.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return users, nil
}
