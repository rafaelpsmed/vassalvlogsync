package server

import (
	"context"
	"io"
	"os"
)

type BlobStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

func NewBlobStore() (BlobStore, error) {
	driver := os.Getenv("STORAGE_DRIVER")
	switch driver {
	case "s3":
		return newS3Store()
	default:
		return &LocalBlobStore{dataDir: envOr("DATA_DIR", "./data/vlogs")}, nil
	}
}
