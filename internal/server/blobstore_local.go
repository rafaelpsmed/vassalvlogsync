package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalBlobStore struct {
	dataDir string
}

func (s *LocalBlobStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	fullPath := filepath.Join(s.dataDir, key)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("criar diretório: %w", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("ler dados: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("escrever arquivo: %w", err)
	}
	return nil
}

func (s *LocalBlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.dataDir, key)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("arquivo não encontrado: %s", key)
		}
		return nil, err
	}
	return f, nil
}

func (s *LocalBlobStore) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(s.dataDir, key)
	return os.Remove(fullPath)
}
