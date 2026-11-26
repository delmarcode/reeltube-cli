package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

// Client is the HTTP client for the ReelTube API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new API client
func NewClient(baseURL, apiKey string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// DoRequest performs an HTTP request without context
func (c *Client) DoRequest(method, path string, params interface{}, target interface{}) (*APIResponse, error) {
	return c.DoRequestWithContext(context.Background(), method, path, params, target)
}

// DoRequestWithContext performs an HTTP request with context
func (c *Client) DoRequestWithContext(ctx context.Context, method, path string, params interface{}, target interface{}) (*APIResponse, error) {
	fullURL, _ := url.JoinPath(c.baseURL, path)
	var body io.Reader

	if params != nil {
		jsonData, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(jsonData)
		c.logger.Debug("request body", "body", string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("sending request", "method", req.Method, "url", req.URL.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	c.logger.Debug("response received", "status", resp.Status)

	apiResponse := &APIResponse{
		Status: resp.StatusCode,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Try to parse JSON error response first
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != "" {
			apiResponse.Error = errResp.Error
		} else {
			apiResponse.Error = string(bodyBytes)
		}
		return apiResponse, fmt.Errorf("API error (status %d): %s", resp.StatusCode, apiResponse.Error)
	}

	var respBodyBytes []byte
	respBodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("response body", "body", string(respBodyBytes))

	if err := json.Unmarshal(respBodyBytes, target); err != nil {
		return nil, err
	}

	apiResponse.Data = target
	return apiResponse, nil
}

// Get performs a GET request
func (c *Client) Get(path string, params, target interface{}) (*APIResponse, error) {
	return c.DoRequest("GET", path, params, target)
}

// Post performs a POST request
func (c *Client) Post(path string, params, target interface{}) (*APIResponse, error) {
	return c.DoRequest("POST", path, params, target)
}

// Put performs a PUT request
func (c *Client) Put(path string, params, target interface{}) (*APIResponse, error) {
	return c.DoRequest("PUT", path, params, target)
}

// Delete performs a DELETE request
func (c *Client) Delete(path string, params, target interface{}) (*APIResponse, error) {
	return c.DoRequest("DELETE", path, params, target)
}

//
// Domain Methods
//

// Me retrieves the current user's profile
func (c *Client) Me() (*MeResponse, error) {
	var data MeResponse
	_, err := c.Get("/api/v0/me", nil, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// CreateMediaUpload initiates a multipart upload
func (c *Client) CreateMediaUpload(fileName string, fileSize int64) (*CreateMediaUploadResponse, error) {
	var data CreateMediaUploadResponse
	body := map[string]interface{}{
		"filename": fileName,
		"size":     fileSize,
	}
	_, err := c.Post("/api/v0/media_uploads", body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// CompleteMultipartUpload completes a multipart upload
func (c *Client) CompleteMultipartUpload(mediaUploadID, uploadID string, parts []Part) error {
	var data CompleteUploadResponse
	body := CompleteUploadRequest{
		UploadID: uploadID,
		Parts:    parts,
	}
	_, err := c.Post(fmt.Sprintf("/api/v0/media_uploads/%s/complete", mediaUploadID), body, &data)
	if err != nil {
		return err
	}
	return nil
}

// ListMediaUploads retrieves the user's media uploads
func (c *Client) ListMediaUploads() (*ListMediaUploadsResponse, error) {
	var data ListMediaUploadsResponse
	_, err := c.Get("/api/v0/media_uploads", nil, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// DeleteMediaUpload deletes a media upload by ID
func (c *Client) DeleteMediaUpload(id string) error {
	var data map[string]interface{}
	_, err := c.Delete(fmt.Sprintf("/api/v0/media_uploads/%s", id), nil, &data)
	if err != nil {
		return err
	}
	return nil
}
