package fixtures

import (
	"context"

	. "github.com/onsi/gomega"

	"github.com/telnet2/mysql-vfs/pkg/storage"
	_ "gocloud.dev/blob/memblob" // Register mem:// driver
)

// TestS3 manages an in-memory storage for testing
type TestS3 struct {
	Storage storage.Storage
	ctx     context.Context
}

// NewTestS3 creates an in-memory storage for testing (using gocloud.dev mem:// driver)
func NewTestS3() *TestS3 {
	ctx := context.Background()

	// Use in-memory storage for tests (faster than LocalStack)
	stor, err := storage.NewStorage(ctx, storage.Config{
		BucketURL: "mem://",
	})
	Expect(err).NotTo(HaveOccurred())

	return &TestS3{
		Storage: stor,
		ctx:     ctx,
	}
}

// Reset clears the storage (not needed for mem:// as each test gets new instance)
func (ts *TestS3) Reset() {
	// No-op for in-memory storage
}

// Cleanup closes the storage
func (ts *TestS3) Cleanup() {
	if ts.Storage != nil {
		_ = ts.Storage.Close()
	}
}
