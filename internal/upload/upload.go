package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/the-del-mar-code-company/reeltube-cli/internal/client"
)

const defaultConcurrency = 5

var (
	// AllowedExtensions is the set of allowed file extensions
	AllowedExtensions = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
	}
	// AllowedMIMEs is the set of allowed MIME types
	AllowedMIMEs = map[string]bool{
		"image/jpeg": true, "image/png": true, "image/gif": true,
		"video/mp4": true, "video/quicktime": true, "video/x-msvideo": true, "video/x-matroska": true,
	}
)

// FileValidation holds the results of file validation
type FileValidation struct {
	AbsPath    string
	FileName   string
	Extension  string
	MIMEType   string
	Size       int64
	UploadName string
}

// ValidateFile validates a file for upload and returns file information
func ValidateFile(filePath, uploadName string) (*FileValidation, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	// Ensure the file path is absolute
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	// Check if the file exists and get file info
	fileInfo, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", absPath)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to access file: %w", err)
	}

	// Check the file extension
	ext := strings.ToLower(filepath.Ext(absPath))
	if !AllowedExtensions[ext] {
		return nil, fmt.Errorf("file type not allowed: %s", ext)
	}

	// Open the file to check the MIME type
	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file: %w", err)
	}
	defer file.Close()

	// Check the MIME type
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("unable to read file: %w", err)
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = http.DetectContentType(buffer)
	}
	if !AllowedMIMEs[mimeType] {
		return nil, fmt.Errorf("MIME type not allowed: %s", mimeType)
	}

	// Determine upload name
	finalUploadName := uploadName
	if finalUploadName == "" {
		finalUploadName = filepath.Base(absPath)
	}

	return &FileValidation{
		AbsPath:    absPath,
		FileName:   filepath.Base(absPath),
		Extension:  ext,
		MIMEType:   mimeType,
		Size:       fileInfo.Size(),
		UploadName: finalUploadName,
	}, nil
}

// FormatBytes returns a human-readable string for bytes
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func systemConcurrency() int {
	numCores := runtime.NumCPU()
	if numCores > 1 {
		return numCores
	}
	return defaultConcurrency
}

// MultipartUpload performs a multipart upload of a file
func MultipartUpload(ctx context.Context, c *client.Client, filePath, fileName string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()
	concurrency := systemConcurrency()

	// Step 1: Get presigned URLs from API
	createMediaUploadResp, err := c.CreateMediaUpload(ctx, fileName, fileSize)
	if err != nil {
		return fmt.Errorf("failed to get presigned URLs: %w", err)
	}

	mediaUpload := createMediaUploadResp.MediaUpload
	partSize := createMediaUploadResp.PartSize
	numParts := createMediaUploadResp.NumParts
	uploadID := createMediaUploadResp.UploadID
	presignedURLs := createMediaUploadResp.PresignedURLs

	// Step 2: Upload each part using the presigned URLs with a worker pool
	// Create a cancellable context for fail-fast behavior
	uploadCtx, cancelUpload := context.WithCancel(ctx)
	defer cancelUpload()

	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error
	parts := make([]client.Part, len(presignedURLs))

	jobs := make(chan int, numParts)

	// Initialize progress bar
	bar := pb.StartNew(numParts)
	startTime := time.Now()

	// Worker pool
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for partNum := range jobs {
				// Check for cancellation before starting work
				select {
				case <-uploadCtx.Done():
					return
				default:
				}

				url := presignedURLs[partNum]
				partOffset := int64(partNum) * int64(partSize)
				buffer := make([]byte, partSize)

				// Open a new file descriptor for each goroutine
				file, err := os.Open(filePath)
				if err != nil {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("failed to open file: %w", err)
						cancelUpload()
					})
					return
				}

				_, err = file.Seek(partOffset, io.SeekStart)
				if err != nil {
					file.Close()
					errOnce.Do(func() {
						firstErr = fmt.Errorf("failed to seek file for part %d: %w", partNum, err)
						cancelUpload()
					})
					return
				}

				// Adjust read size for the last part
				readSize := int64(partSize)
				if partOffset+readSize > fileSize {
					readSize = fileSize - partOffset
				}

				n, err := file.Read(buffer[:int(readSize)])
				if err != nil && err != io.EOF {
					file.Close()
					errOnce.Do(func() {
						firstErr = fmt.Errorf("failed to read file for part %d: %w", partNum, err)
						cancelUpload()
					})
					return
				}
				file.Close()

				// Upload with retry logic (uses uploadCtx for cancellation)
				resp, err := UploadPartWithRetry(uploadCtx, url, buffer[:n])
				if err != nil {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("failed to upload part %d: %w", partNum, err)
						cancelUpload()
					})
					return
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("failed to upload part %d: status %d", partNum, resp.StatusCode)
						cancelUpload()
					})
					return
				}

				parts[partNum] = client.Part{
					PartNumber: partNum + 1,
					ETag:       json.RawMessage(resp.Header.Get("ETag")),
				}

				// Update progress bar
				bar.Increment()
				elapsed := time.Since(startTime)
				remaining := time.Duration((numParts-partNum-1)*int(elapsed.Seconds()/float64(partNum+1))) * time.Second
				bar.Set("remaining", fmt.Sprintf("ETA: %s", remaining))
			}
		}()
	}

	// Send jobs to the worker pool
	for i := 0; i < numParts; i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	bar.Finish()

	// Check if any worker encountered an error
	if firstErr != nil {
		return firstErr
	}

	// Step 3: Complete multipart upload
	err = c.CompleteMultipartUpload(ctx, mediaUpload.ID, uploadID, parts)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}
