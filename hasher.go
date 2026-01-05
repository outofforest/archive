package archive

import (
	"crypto/sha256"
	"hash"
	"io"
	"strings"

	"github.com/pkg/errors"
)

// Hasher returns hash implementation, reader and checksum based on identifier in checksum.
func Hasher(checksum string, reader io.Reader) (hash.Hash, io.Reader, string, error) {
	parts := strings.SplitN(checksum, ":", 2)
	if len(parts) != 2 {
		return nil, nil, "", errors.Errorf("incorrect checksum format: %s", checksum)
	}
	hashAlgorithm := parts[0]
	checksum = parts[1]

	var hasher hash.Hash
	switch hashAlgorithm {
	case "sha256":
		hasher = sha256.New()
	default:
		return nil, nil, "", errors.Errorf("unsupported hashing algorithm: %s", hashAlgorithm)
	}

	return hasher, io.TeeReader(reader, hasher), strings.ToLower(checksum), nil
}
