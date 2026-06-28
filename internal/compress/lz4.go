package compress

import (
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
)

type Compressor interface {
	Compress(dst, src []byte) (int, error)
	Decompress(dst, src []byte) (int, error)
}

type LZ4Compressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	mu      sync.RWMutex
	logger  *zap.Logger
}

func NewLZ4Compressor(logger *zap.Logger) (*LZ4Compressor, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(2),
	)
	if err != nil {
		return nil, err
	}

	decoder, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(2),
	)
	if err != nil {
		encoder.Close()
		return nil, err
	}

	return &LZ4Compressor{
		encoder: encoder,
		decoder: decoder,
		logger:  logger,
	}, nil
}

func (c *LZ4Compressor) Compress(dst, src []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	compressed := c.encoder.EncodeAll(src, dst[:0])
	return len(compressed), nil
}

func (c *LZ4Compressor) Decompress(dst, src []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	decompressed, err := c.decoder.DecodeAll(src, dst[:0])
	if err != nil {
		return 0, err
	}
	return len(decompressed), nil
}

func (c *LZ4Compressor) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.encoder.Close(); err != nil {
		return err
	}
	c.decoder.Close()
	return nil
}

type CompressedReader struct {
	reader     io.Reader
	compressor *LZ4Compressor
	buf        []byte
	offset     int
	len        int
}

func NewCompressedReader(reader io.Reader, compressor *LZ4Compressor) *CompressedReader {
	return &CompressedReader{
		reader:     reader,
		compressor: compressor,
		buf:        make([]byte, 64*1024),
	}
}

func (r *CompressedReader) Read(p []byte) (int, error) {
	if r.offset >= r.len {
		n, err := r.reader.Read(r.buf)
		if err != nil {
			return 0, err
		}

		r.len, err = r.compressor.Decompress(r.buf, r.buf[:n])
		if err != nil {
			return 0, err
		}
		r.offset = 0
	}

	n := copy(p, r.buf[r.offset:r.len])
	r.offset += n
	return n, nil
}

type CompressedWriter struct {
	writer     io.Writer
	compressor *LZ4Compressor
	buf        []byte
}

func NewCompressedWriter(writer io.Writer, compressor *LZ4Compressor) *CompressedWriter {
	return &CompressedWriter{
		writer:     writer,
		compressor: compressor,
		buf:        make([]byte, 64*1024),
	}
}

func (w *CompressedWriter) Write(p []byte) (int, error) {
	n, err := w.compressor.Compress(w.buf, p)
	if err != nil {
		return 0, err
	}

	_, err = w.writer.Write(w.buf[:n])
	return len(p), err
}

func (w *CompressedWriter) Close() error {
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type NoopCompressor struct{}

func NewNoopCompressor() *NoopCompressor {
	return &NoopCompressor{}
}

func (c *NoopCompressor) Compress(dst, src []byte) (int, error) {
	return copy(dst, src), nil
}

func (c *NoopCompressor) Decompress(dst, src []byte) (int, error) {
	return copy(dst, src), nil
}

func (c *NoopCompressor) Close() error {
	return nil
}
