package client

import "encoding/json"

// APIResponse wraps API responses with status and error info
type APIResponse struct {
	Data   interface{}
	Error  string
	Status int // HTTP status code
}

// MeResponse is the response from the /me endpoint
type MeResponse struct {
	Profile struct {
		ID         string  `json:"id"`
		Handle     string  `json:"handle"`
		Bio        *string `json:"bio"`
		ImagePath  *string `json:"image_path"`
		IsBusiness bool    `json:"is_business"`
		IsVerified bool    `json:"is_verified"`
	} `json:"profile"`
}

// MediaUpload represents a media upload
type MediaUpload struct {
	ID         string  `json:"id"`
	FileName   string  `json:"filename"`
	Size       int64   `json:"size"`
	MIMEType   *string `json:"mimetype"`
	Status     string  `json:"status"`
	InsertedAt string  `json:"inserted_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// CreateMediaUploadResponse is the response from creating a media upload
type CreateMediaUploadResponse struct {
	UploadID      string      `json:"upload_id"`
	PartSize      int         `json:"part_size"`
	NumParts      int         `json:"num_parts"`
	PresignedURLs []string    `json:"presigned_urls"`
	MediaUpload   MediaUpload `json:"media_upload"`
}

// Part represents an uploaded part with its ETag
type Part struct {
	PartNumber int             `json:"part_number"`
	ETag       json.RawMessage `json:"etag"`
}

// CompleteUploadRequest is the request to complete a multipart upload
// Note: media_upload_id is passed in the URL, not the body
type CompleteUploadRequest struct {
	UploadID string `json:"upload_id"`
	Parts    []Part `json:"parts"`
}

// CompleteUploadResponse is the response from completing a multipart upload
type CompleteUploadResponse struct {
	MediaUpload MediaUpload `json:"media_upload"`
}

// MediaUploadItem represents a media upload in the list response
type MediaUploadItem struct {
	ID        string  `json:"id"`
	FileName  string  `json:"filename"`
	Size      int64   `json:"size"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	URL       *string `json:"url"`
}

// ListMediaUploadsResponse is the response from listing media uploads
type ListMediaUploadsResponse struct {
	MediaUploads []MediaUploadItem `json:"media_uploads"`
}
