package client

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testLogger returns a logger that discards output (for tests)
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com", "test-key", nil)

	if client.baseURL != "https://api.example.com" {
		t.Errorf("expected baseURL to be 'https://api.example.com', got '%s'", client.baseURL)
	}
	if client.apiKey != "test-key" {
		t.Errorf("expected apiKey to be 'test-key', got '%s'", client.apiKey)
	}
	if client.logger == nil {
		t.Error("expected logger to be initialized even when nil passed")
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
}

func TestNewClientWithLogger(t *testing.T) {
	logger := testLogger()
	client := NewClient("https://api.example.com", "test-key", logger)

	if client.logger != logger {
		t.Error("expected logger to be the provided logger")
	}
}

func TestDoRequestSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Authorization 'Bearer test-key', got '%s'", auth)
		}

		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "success"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	var response map[string]string
	apiResp, err := client.DoRequest("GET", "/test", nil, &response)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if apiResp.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", apiResp.Status)
	}
	if response["message"] != "success" {
		t.Errorf("expected message 'success', got '%s'", response["message"])
	}
}

func TestDoRequest4xxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	var response map[string]string
	apiResp, err := client.DoRequest("GET", "/test", nil, &response)

	if err == nil {
		t.Fatal("expected error for 4xx response, got nil")
	}
	if apiResp == nil {
		t.Fatal("expected apiResponse to be returned even on error")
	}
	if apiResp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", apiResp.Status)
	}
	if apiResp.Error != "bad request" {
		t.Errorf("expected error 'bad request', got '%s'", apiResp.Error)
	}
}

func TestDoRequestJSONErrorParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid API key"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	var response map[string]string
	apiResp, err := client.DoRequest("GET", "/test", nil, &response)

	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	if apiResp.Error != "Invalid API key" {
		t.Errorf("expected error 'Invalid API key', got '%s'", apiResp.Error)
	}
}

func TestDoRequestNonJSONErrorFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`Service temporarily unavailable`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	var response map[string]string
	apiResp, err := client.DoRequest("GET", "/test", nil, &response)

	if err == nil {
		t.Fatal("expected error for 503 response, got nil")
	}
	if apiResp.Error != "Service temporarily unavailable" {
		t.Errorf("expected error 'Service temporarily unavailable', got '%s'", apiResp.Error)
	}
}

func TestMe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/me" {
			t.Errorf("expected path '/api/v0/me', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected method GET, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"profile": map[string]interface{}{
				"id":          "123",
				"handle":      "testuser",
				"is_business": false,
				"is_verified": true,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	me, err := client.Me()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if me.Profile.ID != "123" {
		t.Errorf("expected ID '123', got '%s'", me.Profile.ID)
	}
	if me.Profile.Handle != "testuser" {
		t.Errorf("expected handle 'testuser', got '%s'", me.Profile.Handle)
	}
}

func TestCreateMediaUpload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/media_uploads" {
			t.Errorf("expected path '/api/v0/media_uploads', got '%s'", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected method POST, got '%s'", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["filename"] != "test.mp4" {
			t.Errorf("expected filename 'test.mp4', got '%v'", body["filename"])
		}
		if body["size"].(float64) != 1024 {
			t.Errorf("expected size 1024, got '%v'", body["size"])
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"upload_id":      "upload-123",
			"part_size":      5242880,
			"num_parts":      1,
			"presigned_urls": []string{"https://s3.example.com/part1"},
			"media_upload":   map[string]string{"id": "media-123"},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	resp, err := client.CreateMediaUpload("test.mp4", 1024)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.UploadID != "upload-123" {
		t.Errorf("expected upload_id 'upload-123', got '%s'", resp.UploadID)
	}
	if resp.MediaUpload.ID != "media-123" {
		t.Errorf("expected media_upload.id 'media-123', got '%s'", resp.MediaUpload.ID)
	}
}

func TestListMediaUploads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/media_uploads" {
			t.Errorf("expected path '/api/v0/media_uploads', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected method GET, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"media_uploads": []map[string]interface{}{
				{
					"id":         "upload-1",
					"filename":   "test.mp4",
					"size":       1024,
					"status":     "completed",
					"created_at": "2024-01-01T00:00:00Z",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	resp, err := client.ListMediaUploads()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.MediaUploads) != 1 {
		t.Errorf("expected 1 upload, got %d", len(resp.MediaUploads))
	}
	if resp.MediaUploads[0].ID != "upload-1" {
		t.Errorf("expected upload ID 'upload-1', got '%s'", resp.MediaUploads[0].ID)
	}
}

func TestDeleteMediaUpload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/media_uploads/upload-123" {
			t.Errorf("expected path '/api/v0/media_uploads/upload-123', got '%s'", r.URL.Path)
		}
		if r.Method != "DELETE" {
			t.Errorf("expected method DELETE, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", nil)

	err := client.DeleteMediaUpload("upload-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
