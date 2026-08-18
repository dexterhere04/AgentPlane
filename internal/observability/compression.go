package observability

import (
    "bytes"
    "compress/gzip"
    "io"
)

func CompressPayload(src []byte) ([]byte, error) {
    var buf bytes.Buffer
    gw := gzip.NewWriter(&buf)
    if _, err := gw.Write(src); err != nil {
        gw.Close()
        return nil, err
    }
    if err := gw.Close(); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

func DecompressPayload(src []byte) ([]byte, error) {
    gr, err := gzip.NewReader(bytes.NewReader(src))
    if err != nil {
        return nil, err
    }
    defer gr.Close()
    var out bytes.Buffer
    if _, err := io.Copy(&out, gr); err != nil {
        return nil, err
    }
    return out.Bytes(), nil
}
