package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gocloud.dev/blob"
)

const (
	storageModeInlineJSON = "inline_json"
	storageModeBlob       = "blob"
	storageModeS3Blob     = "s3_blob"
)

type StoreRequest struct {
	Data     []byte
	FileName string
	MimeType string
}

type StoreResult struct {
	StorageMode string
	BlobKey     *string
	JSONPayload []byte
	Checksum    string
	Size        int64
	MimeType    string
}

type LoadResult struct {
	Data     []byte
	Checksum string
	Size     int64
}

type StorageService struct {
	Bucket              *blob.Bucket
	InlineJSONMaxBytes  int64
	InlineJSONMediaType map[string]struct{}
	BucketURL           string
}

func NewStorageService(bucket *blob.Bucket, bucketURL string, inlineMax int64, inlineMedia []string) *StorageService {
	mt := make(map[string]struct{}, len(inlineMedia))
	for _, item := range inlineMedia {
		if trimmed := strings.TrimSpace(strings.ToLower(item)); trimmed != "" {
			mt[trimmed] = struct{}{}
		}
	}
	return &StorageService{
		Bucket:              bucket,
		BucketURL:           bucketURL,
		InlineJSONMaxBytes:  inlineMax,
		InlineJSONMediaType: mt,
	}
}

func (s *StorageService) Store(ctx context.Context, req StoreRequest) (StoreResult, error) {
	size := int64(len(req.Data))
	checksum := computeChecksum(req.Data)
	mimeType := strings.TrimSpace(req.MimeType)

	if jsonPayload, ok := s.tryInlineJSON(req.Data, size, mimeType); ok {
		if mimeType == "" {
			mimeType = "application/json"
		}
		return StoreResult{
			StorageMode: storageModeInlineJSON,
			JSONPayload: jsonPayload,
			Checksum:    checksum,
			Size:        size,
			MimeType:    mimeType,
		}, nil
	}

	blobKey, err := s.writeBlob(ctx, req.Data, req.FileName)
	if err != nil {
		return StoreResult{}, err
	}

	// Determine storage mode based on bucket URL
	storageMode := storageModeBlob
	if strings.HasPrefix(s.BucketURL, "s3://") {
		storageMode = storageModeS3Blob
	}

	return StoreResult{
		StorageMode: storageMode,
		BlobKey:     &blobKey,
		Checksum:    checksum,
		Size:        size,
		MimeType:    mimeType,
	}, nil
}

func (s *StorageService) Load(ctx context.Context, blobKey string) (LoadResult, error) {
	reader, err := s.Bucket.NewReader(ctx, blobKey, nil)
	if err != nil {
		return LoadResult{}, err
	}
	defer reader.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(reader); err != nil {
		return LoadResult{}, err
	}
	data := buf.Bytes()
	return LoadResult{
		Data:     data,
		Checksum: computeChecksum(data),
		Size:     int64(len(data)),
	}, nil
}

func (s *StorageService) tryInlineJSON(data []byte, size int64, mimeType string) ([]byte, bool) {
	if size == 0 || size > s.InlineJSONMaxBytes {
		return nil, false
	}
	if mimeType != "" {
		mt := strings.ToLower(mimeType)
		if _, ok := s.InlineJSONMediaType[mt]; !ok {
			// attempt to normalize and compare without parameters, e.g., application/json; charset=utf-8
			if m, _, err := mime.ParseMediaType(mt); err == nil {
				if _, ok := s.InlineJSONMediaType[strings.ToLower(m)]; !ok {
					return nil, false
				}
			}
		}
	}
	if !json.Valid(data) {
		return nil, false
	}
	compact := &bytes.Buffer{}
	if err := json.Compact(compact, data); err != nil {
		return nil, false
	}
	return compact.Bytes(), true
}

func (s *StorageService) writeBlob(ctx context.Context, data []byte, name string) (string, error) {
	key := buildBlobKey(name)
	writer, err := s.Bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return "", err
	}
	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return key, nil
}

func buildBlobKey(name string) string {
	uid := uuid.NewString()
	clean := strings.TrimSpace(name)
	ext := ""
	if clean != "" {
		ext = strings.ToLower(filepath.Ext(clean))
	}
	return fmt.Sprintf("files/%s/%s%s", time.Now().UTC().Format("20060102"), uid, ext)
}

func computeChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
