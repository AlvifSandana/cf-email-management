package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
)

// WorkerSummary represents metadata for a Cloudflare Worker script.
type WorkerSummary struct {
	ID         string    `json:"id"`
	ETag       string    `json:"etag"`
	CreatedOn  time.Time `json:"created_on"`
	ModifiedOn time.Time `json:"modified_on"`
}

// ListWorkers retrieves all worker scripts for the specified Cloudflare account.
// Endpoint: GET /accounts/{account_id}/workers/scripts
func (c *Client) ListWorkers(ctx context.Context, accountID string) ([]WorkerSummary, error) {
	var resp cfResponse[[]WorkerSummary]
	path := fmt.Sprintf("/accounts/%s/workers/scripts", accountID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, c.formatCFError(resp.Errors)
	}
	if resp.Result == nil {
		return []WorkerSummary{}, nil
	}
	return resp.Result, nil
}

// UploadWorker uploads or updates a worker script for the specified Cloudflare account.
// Endpoint: PUT /accounts/{account_id}/workers/scripts/{script_name} with Content-Type: application/javascript
func (c *Client) UploadWorker(ctx context.Context, accountID, scriptName, scriptContent string) error {
	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", accountID, scriptName)
	reqURL := fmt.Sprintf("%s%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, strings.NewReader(scriptContent))
	if err != nil {
		return domain.NewInternalError("failed to create http request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/javascript")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return domain.NewProviderTimeoutError("provider request timed out", err)
		}
		return domain.NewProviderError("provider connection error", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProviderError("failed to read provider response", err)
	}

	var cfResp cfResponse[any]
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &cfResp); err != nil {
			return domain.NewProviderError(fmt.Sprintf("failed to parse provider response: %s", string(respBytes)), err)
		}
	}

	if resp.StatusCode >= 400 {
		return domain.NewProviderError(fmt.Sprintf("provider returned status %d: %s", resp.StatusCode, string(respBytes)), nil)
	}

	if !cfResp.Success {
		return c.formatCFError(cfResp.Errors)
	}

	return nil
}

// DeleteWorker deletes a worker script for the specified Cloudflare account.
// Endpoint: DELETE /accounts/{account_id}/workers/scripts/{script_name}
func (c *Client) DeleteWorker(ctx context.Context, accountID, scriptName string) error {
	var resp cfResponse[any]
	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", accountID, scriptName)
	if err := c.doRequest(ctx, http.MethodDelete, path, nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return c.formatCFError(resp.Errors)
	}
	return nil
}
