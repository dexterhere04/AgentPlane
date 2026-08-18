package observability

import (
    "bytes"
    "testing"
)

func TestCompressDecompress(t *testing.T) {
    data := bytes.Repeat([]byte("hello world "), 100)
    c, err := CompressPayload(data)
    if err != nil {
        t.Fatalf("compress error: %v", err)
    }
    d, err := DecompressPayload(c)
    if err != nil {
        t.Fatalf("decompress error: %v", err)
    }
    if !bytes.Equal(d, data) {
        t.Fatalf("decompressed data mismatch")
    }
}
