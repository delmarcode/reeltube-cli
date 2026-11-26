package upload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/the-del-mar-code-company/reeltube-cli/internal/client"
)

func TestSystemConcurrency(t *testing.T) {
	result := systemConcurrency()

	numCPU := runtime.NumCPU()
	if numCPU > 1 {
		if result != numCPU {
			t.Errorf("expected %d (NumCPU), got %d", numCPU, result)
		}
	} else {
		if result != defaultConcurrency {
			t.Errorf("expected defaultConcurrency %d for single CPU, got %d", defaultConcurrency, result)
		}
	}
}

func TestAllowedExtensions(t *testing.T) {
	testCases := []struct {
		ext      string
		expected bool
	}{
		{".jpg", true},
		{".jpeg", true},
		{".png", true},
		{".gif", true},
		{".mp4", true},
		{".mov", true},
		{".avi", true},
		{".mkv", true},
		{".exe", false},
		{".txt", false},
		{".pdf", false},
		{".webm", false},
	}

	for _, tc := range testCases {
		result := AllowedExtensions[tc.ext]
		if result != tc.expected {
			t.Errorf("extension %s: expected %v, got %v", tc.ext, tc.expected, result)
		}
	}
}

func TestAllowedMIMETypes(t *testing.T) {
	testCases := []struct {
		mime     string
		expected bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"video/mp4", true},
		{"video/quicktime", true},
		{"video/x-msvideo", true},
		{"video/x-matroska", true},
		{"application/pdf", false},
		{"text/plain", false},
		{"video/webm", false},
	}

	for _, tc := range testCases {
		result := AllowedMIMEs[tc.mime]
		if result != tc.expected {
			t.Errorf("MIME %s: expected %v, got %v", tc.mime, tc.expected, result)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		statusCode int
		expected   bool
	}{
		{"context canceled", context.Canceled, 0, false},
		{"context deadline exceeded", context.DeadlineExceeded, 0, false},
		{"500 server error", nil, 500, true},
		{"502 bad gateway", nil, 502, true},
		{"503 service unavailable", nil, 503, true},
		{"400 bad request", nil, 400, false},
		{"401 unauthorized", nil, 401, false},
		{"404 not found", nil, 404, false},
		{"200 ok", nil, 200, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsRetryableError(tc.err, tc.statusCode)
			if result != tc.expected {
				t.Errorf("IsRetryableError(%v, %d) = %v, expected %v", tc.err, tc.statusCode, result, tc.expected)
			}
		})
	}
}

func TestUploadPartWithRetrySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := UploadPartWithRetry(context.Background(), server.URL, []byte("test data"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestUploadPartWithRetryRecoversFrom5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptNum := atomic.AddInt32(&attempts, 1)
		if attemptNum <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := UploadPartWithRetry(context.Background(), server.URL, []byte("test data"))
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestUploadPartWithRetryMaxRetriesExceeded(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := UploadPartWithRetry(context.Background(), server.URL, []byte("test data"))
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	if atomic.LoadInt32(&attempts) != 4 {
		t.Errorf("expected 4 attempts, got %d", attempts)
	}
}

func TestUploadPartWithRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := UploadPartWithRetry(ctx, "http://localhost:9999", []byte("test data"))
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if err != context.Canceled && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestUploadPartWithRetryNo4xxRetry(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	resp, err := UploadPartWithRetry(context.Background(), server.URL, []byte("test data"))
	// 4xx errors should fail immediately without retry
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error for 4xx status, got nil")
	}

	if !strings.Contains(err.Error(), "client error") || !strings.Contains(err.Error(), "400") {
		t.Errorf("expected client error with status 400, got: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry for 4xx), got %d", attempts)
	}
}

func TestUploadPartWithRetry401Unauthorized(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	resp, err := UploadPartWithRetry(context.Background(), server.URL, []byte("test data"))
	// 401 should fail immediately without retry
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error for 401 status, got nil")
	}

	if !strings.Contains(err.Error(), "client error") || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected client error with status 401, got: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry for 401), got %d", attempts)
	}
}

// createTestFile creates a temporary test file with the specified size
func createTestFile(t *testing.T, size int) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.mp4")

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	return tmpFile
}

func TestMultipartUploadIntegration(t *testing.T) {
	testFile := createTestFile(t, 1024*100)

	var createCalled, completeCalled int32
	var partUploads int32

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT method, got %s", r.Method)
		}
		atomic.AddInt32(&partUploads, 1)
		w.Header().Set("ETag", `"mock-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v0/media_uploads" && r.Method == "POST":
			atomic.AddInt32(&createCalled, 1)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"upload_id": "upload-123",
				"part_size": 50 * 1024,
				"num_parts": 2,
				"presigned_urls": []string{
					s3Server.URL + "/part1",
					s3Server.URL + "/part2",
				},
				"media_upload": map[string]string{"id": "media-123"},
			})
		case r.URL.Path == "/api/v0/media_uploads/media-123/complete" && r.Method == "POST":
			atomic.AddInt32(&completeCalled, 1)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"media_upload": map[string]string{"id": "media-123"},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	c := client.NewClient(apiServer.URL, "test-key", nil)

	err := MultipartUpload(context.Background(), c, testFile, "test.mp4")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if atomic.LoadInt32(&createCalled) != 1 {
		t.Errorf("expected CreateMediaUpload to be called once, got %d", createCalled)
	}
	if atomic.LoadInt32(&completeCalled) != 1 {
		t.Errorf("expected CompleteMultipartUpload to be called once, got %d", completeCalled)
	}
	if atomic.LoadInt32(&partUploads) != 2 {
		t.Errorf("expected 2 part uploads, got %d", partUploads)
	}
}

func TestMultipartUploadContextCancellation(t *testing.T) {
	testFile := createTestFile(t, 1024*100)

	var partUploads int32

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&partUploads, 1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("ETag", `"mock-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/media_uploads" && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"upload_id": "upload-123",
				"part_size": 25 * 1024,
				"num_parts": 4,
				"presigned_urls": []string{
					s3Server.URL + "/part1",
					s3Server.URL + "/part2",
					s3Server.URL + "/part3",
					s3Server.URL + "/part4",
				},
				"media_upload": map[string]string{"id": "media-123"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	c := client.NewClient(apiServer.URL, "test-key", nil)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := MultipartUpload(ctx, c, testFile, "test.mp4")

	if err == nil {
		t.Log("Warning: Upload completed before cancellation")
	}
}

func TestMultipartUploadPartialFailure(t *testing.T) {
	testFile := createTestFile(t, 1024*100)

	var partUploads int32

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&partUploads, 1)
		if count == 2 || count == 3 || count == 4 || count == 5 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("ETag", `"mock-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/media_uploads" && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"upload_id": "upload-123",
				"part_size": 50 * 1024,
				"num_parts": 2,
				"presigned_urls": []string{
					s3Server.URL + "/part1",
					s3Server.URL + "/part2",
				},
				"media_upload": map[string]string{"id": "media-123"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	c := client.NewClient(apiServer.URL, "test-key", nil)

	err := MultipartUpload(context.Background(), c, testFile, "test.mp4")

	if err == nil {
		t.Fatal("expected error due to part failure, got nil")
	}
}

func TestValidateFile(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "test.mp4")
	if err := os.WriteFile(validFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	validation, err := ValidateFile(validFile, "")
	if err != nil {
		t.Errorf("expected no error for valid file, got %v", err)
	}
	if validation.Extension != ".mp4" {
		t.Errorf("expected extension '.mp4', got '%s'", validation.Extension)
	}
	if validation.UploadName != "test.mp4" {
		t.Errorf("expected upload name 'test.mp4', got '%s'", validation.UploadName)
	}

	validation, err = ValidateFile(validFile, "custom.mp4")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if validation.UploadName != "custom.mp4" {
		t.Errorf("expected upload name 'custom.mp4', got '%s'", validation.UploadName)
	}

	_, err = ValidateFile("", "")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}

	_, err = ValidateFile("/nonexistent/file.mp4", "")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	invalidFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(invalidFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	_, err = ValidateFile(invalidFile, "")
	if err == nil {
		t.Error("expected error for invalid extension, got nil")
	}
}

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tc := range testCases {
		result := FormatBytes(tc.bytes)
		if result != tc.expected {
			t.Errorf("FormatBytes(%d) = '%s', expected '%s'", tc.bytes, result, tc.expected)
		}
	}
}
