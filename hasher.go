package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"strings"

	"github.com/pkg/errors"
)

var _ io.Reader = &HashingReader{}

// HashingReader reads stream, computes and verifies checksum.
type HashingReader struct {
	reader   io.Reader
	hasher   hash.Hash
	checksum string
}

// NewHashingReader creates new hashing reader.
func NewHashingReader(reader io.Reader, checksum string) (*HashingReader, error) {
	parts := strings.SplitN(checksum, ":", 2)
	if len(parts) != 2 {
		return nil, errors.Errorf("incorrect checksum format: %s", checksum)
	}
	hashAlgorithm := parts[0]
	checksum = parts[1]

	var hasher hash.Hash
	switch hashAlgorithm {
	case "sha256":
		hasher = sha256.New()
	default:
		return nil, errors.Errorf("unsupported hashing algorithm: %s", hashAlgorithm)
	}

	return &HashingReader{
		reader:   io.TeeReader(reader, hasher),
		hasher:   hasher,
		checksum: checksum,
	}, nil
}

// Read reads bytes from stream.
func (hr *HashingReader) Read(p []byte) (int, error) {
	n, err := hr.reader.Read(p)

	switch {
	case err == nil:
		return n, nil
	case errors.Is(err, io.EOF):
		return n, io.EOF
	default:
		return 0, errors.WithStack(err)
	}
}

// ValidateChecksum validates checksum.
// Before validating it reads all the remaining bytes from stream to ensure that checksum is computed
// from all bytes.
func (hr *HashingReader) ValidateChecksum() error {
	if _, err := io.Copy(io.Discard, hr.reader); err != nil {
		return errors.WithStack(err)
	}

	checksum := hex.EncodeToString(hr.hasher.Sum(nil))
	if checksum != hr.checksum {
		return errors.Errorf("checksum mismatch, expected: %q, got: %q", hr.checksum, checksum)
	}
	return nil
}
