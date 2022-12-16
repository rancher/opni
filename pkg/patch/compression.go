package patch

import (
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc/encoding"
)

type compressor struct {
	encoderPool sync.Pool
	decoderPool sync.Pool
}

func (c *compressor) Name() string {
	return "zstd"
}

func init() {
	c := &compressor{}
	c.encoderPool.New = func() any {
		w, _ := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedBetterCompression),
			zstd.WithNoEntropyCompression(true),
		)
		return &writer{
			Encoder: w,
			pool:    &c.encoderPool,
		}
	}
	encoding.RegisterCompressor(c)
}

type writer struct {
	*zstd.Encoder
	pool *sync.Pool
}

func (c *compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	z := c.encoderPool.Get().(*writer)
	z.Encoder.Reset(w)
	return z, nil
}

func (z *writer) Close() error {
	defer z.pool.Put(z)
	return z.Encoder.Close()
}

type reader struct {
	*zstd.Decoder
	pool *sync.Pool
}

func (c *compressor) Decompress(r io.Reader) (io.Reader, error) {
	z, inPool := c.decoderPool.Get().(*reader)
	if !inPool {
		newZ, err := zstd.NewReader(r,
			zstd.WithDecoderLowmem(true),
			zstd.WithDecoderMaxMemory(32*1024*1024), // 32MB
		)
		if err != nil {
			return nil, err
		}
		return &reader{
			Decoder: newZ,
			pool:    &c.decoderPool,
		}, nil
	}
	if err := z.Reset(r); err != nil {
		c.decoderPool.Put(z)
		return nil, err
	}
	return z, nil
}

func (z *reader) Read(p []byte) (n int, err error) {
	n, err = z.Decoder.Read(p)
	if err == io.EOF {
		z.pool.Put(z)
	}
	return n, err
}
