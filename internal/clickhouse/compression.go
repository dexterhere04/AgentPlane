package clickhouse

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// CompressText compresses text using gzip. Returns the compressed bytes
// (as a binary-safe string, suitable for ClickHouse's String column type),
// along with the original and compressed sizes.
func CompressText(text string) (string, uint32, uint32, error) {
	original := []byte(text)
	originalSize := uint32(len(original))

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(original); err != nil {
		return "", 0, 0, err
	}
	if err := w.Close(); err != nil {
		return "", 0, 0, err
	}

	compressedSize := uint32(buf.Len())
	return buf.String(), originalSize, compressedSize, nil
}

// DecompressText decompresses gzip-compressed text
func DecompressText(compressed string) (string, error) {
	r, err := gzip.NewReader(bytes.NewReader([]byte(compressed)))
	if err != nil {
		return "", err
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	return string(decompressed), nil
}

// HashText returns SHA256 hash of text
func HashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
