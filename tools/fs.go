package tools

import (
	"context"
	"os"
)

// FileSystem — абстракция чтения/записи файлов для встроенных инструментов.
type FileSystem interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
}

// OSFileSystem реализует FileSystem через os.ReadFile / os.WriteFile.
type OSFileSystem struct{}

func (OSFileSystem) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OSFileSystem) WriteFile(_ context.Context, path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
