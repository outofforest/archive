package archive

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func ensureDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.WithStack(err)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.WithStack(err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return dir, nil
}

func ensureFileInDir(dir, file string) (string, error) {
	file = filepath.Join(dir, file)

	// It happened in tar archive that it contained ./ entry.
	if file == dir {
		return "", nil
	}

	rel, err := filepath.Rel(dir, file)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.Errorf("file %s is outside base directory %s", file, dir)
	}
	return file, nil
}

func ensureFileDir(file string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
