package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
)

func normalizeExecutablePath(executable string) (string, error) {
	if executable == "" {
		return "", errors.New("process executable path is empty")
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Clean(executable), nil
	}
	return "", err
}
