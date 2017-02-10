package backends

import (
	"bytes"
	"compress/zlib"
	"github.com/flashmob/go-guerrilla/envelope"
	"io"
	"sync"
)

// compressedData struct will be compressed using zlib when printed via fmt
type compressor struct {
	extraHeaders []byte
	data         *bytes.Buffer
	pool         *sync.Pool
}

// newCompressedData returns a new CompressedData
func newCompressor() *compressor {
	var p = sync.Pool{
		New: func() interface{} {
			var b bytes.Buffer
			return &b
		},
	}
	return &compressor{
		pool: &p,
	}
}

// Set the extraheaders and buffer of data to compress
func (c *compressor) set(b []byte, d *bytes.Buffer) {
	c.extraHeaders = b
	c.data = d
}

// implement Stringer interface
func (c *compressor) String() string {
	if c.data == nil {
		return ""
	}
	//borrow a buffer form the pool
	b := c.pool.Get().(*bytes.Buffer)
	// put back in the pool
	defer func() {
		b.Reset()
		c.pool.Put(b)
	}()

	var r *bytes.Reader
	w, _ := zlib.NewWriterLevel(b, zlib.BestSpeed)
	r = bytes.NewReader(c.extraHeaders)
	io.Copy(w, r)
	io.Copy(w, c.data)
	w.Close()
	return b.String()
}

// clear it, without clearing the pool
func (c *compressor) clear() {
	c.extraHeaders = []byte{}
	c.data = nil
}

// The hasher decorator computes a hash of the email for each recipient
// It appends the hashes to envelope's Hashes slice.
func Compressor() Decorator {
	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {

			compressor := newCompressor()
			compressor.set([]byte(e.DeliveryHeader), &e.Data)
			if e.Meta == nil {
				e.Meta = make(map[string]interface{})
			}
			e.Meta["zlib-compressor"] = compressor

			return c.Process(e)
		})
	}
}
