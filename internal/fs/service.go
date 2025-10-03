package fs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/access"
	"github.com/telnet2/mysql-vfs/internal/config"
	"github.com/telnet2/mysql-vfs/internal/models"
)

// ErrAccessDenied is returned when hierarchical access control rejects an operation.
var ErrAccessDenied = errors.New("access denied by policy")

// ErrInvalidTransition indicates a workflow rule rejected a move.
var ErrInvalidTransition = errors.New("invalid workflow transition")

// Service coordinates access to the virtual file system backed by MySQL.
type Service struct {
	db       *gorm.DB
	policies *access.PolicyEngine
	blob     *blob.Bucket
	cfg      config.Config
}

// NewService creates a Service.
func NewService(db *gorm.DB, policies *access.PolicyEngine, cfg config.Config) (*Service, error) {
	bucket, err := blob.OpenBucket(context.Background(), cfg.Blob.URL)
	if err != nil {
		return nil, err
	}

	return &Service{db: db, policies: policies, blob: bucket, cfg: cfg}, nil
}

// Close shuts down resources owned by the service.
func (s *Service) Close() error {
	if s.blob == nil {
		return nil
	}

	return s.blob.Close()
}

// NodeWithPolicy is a helper returned from queries containing only the columns required for access control evaluation.
type NodeWithPolicy struct {
	Path         string
	PolicyScript string
	PolicyHash   string
}

// AuthorizePath ensures that all policies on the path authorize the operation.
func (s *Service) AuthorizePath(ctx context.Context, nodes []NodeWithPolicy, input map[string]any) error {
	for _, node := range nodes {
		allow, err := s.policies.Evaluate(ctx, access.Policy{Script: node.PolicyScript, Hash: node.PolicyHash}, input)
		if err != nil {
			return fmt.Errorf("policy evaluation failed for %s: %w", node.Path, err)
		}

		if !allow {
			return fmt.Errorf("%w: path=%s", ErrAccessDenied, node.Path)
		}
	}

	return nil
}

// ReadNode loads a node including its content, automatically resolving inline vs blob storage.
func (s *Service) ReadNode(ctx context.Context, path string) (*models.Node, []byte, error) {
	var node models.Node

	if err := s.db.WithContext(ctx).Where("path = ? AND deleted = ?", path, false).Take(&node).Error; err != nil {
		return nil, nil, err
	}

	if len(node.ContentInline) > 0 {
		return &node, append([]byte(nil), node.ContentInline...), nil
	}

	if node.ContentURL == "" {
		return &node, nil, nil
	}

	reader, err := s.blob.NewReader(ctx, node.ContentURL, nil)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()

	data := make([]byte, reader.Size())
	if _, err := reader.Read(data); err != nil {
		return nil, nil, err
	}

	return &node, data, nil
}

// WriteContent stores the provided content either inline or via blob storage depending on size.
func (s *Service) WriteContent(ctx context.Context, node *models.Node, content []byte) error {
	const inlineLimit = 1 << 20 // 1MB

	if len(content) <= inlineLimit {
		node.ContentInline = append([]byte(nil), content...)
		node.ContentURL = ""
		return nil
	}

	if node.ContentURL == "" {
		node.ContentURL = fmt.Sprintf("node/%d/%d", time.Now().UnixNano(), node.ID)
	}

	writer, err := s.blob.NewWriter(ctx, node.ContentURL, nil)
	if err != nil {
		return err
	}

	if _, err := writer.Write(content); err != nil {
		writer.Close()
		return err
	}

	return writer.Close()
}
