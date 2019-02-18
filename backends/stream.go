package backends

import (
	"bytes"
	"compress/zlib"
	"io"
)

func init() {
	streamers["compressor"] = func() StreamDecorator {
		return StreamTest()
	}
}

type streamCompressor struct {
	zw *zlib.Writer
}

func newStreamCompressor(w io.Writer) io.Writer {
	sc := new(streamCompressor)
	sc.zw, _ = zlib.NewWriterLevel(w, zlib.BestSpeed)
	return sc
}
func (sc *streamCompressor) Close() error {
	return sc.zw.Close()
}
func (sc *streamCompressor) Write(p []byte) (n int, err error) {
	N, err := sc.zw.Write(p)
	return N, err
}

func newStreamDecompresser(w io.Writer) io.Writer {
	sc := new(streamDecompressor)
	sc.w = w
	sc.pr, sc.pw = io.Pipe()
	go sc.consumer()
	return sc
}

type streamDecompressor struct {
	w  io.Writer
	zr io.ReadCloser

	pr  *io.PipeReader
	pw  *io.PipeWriter
	zr2 io.ReadCloser
}

func (sc *streamDecompressor) Close() error {

	errR := sc.pr.Close()
	errW := sc.pw.Close()
	if err := sc.zr.Close(); err != nil {
		return err
	}
	if errR != nil {
		return errR
	}
	if errW != nil {
		return errW
	}

	return nil
}

func (sc *streamDecompressor) Write(p []byte) (n int, err error) {

	N, err := io.Copy(sc.pw, bytes.NewReader(p))
	if N > 0 {
		n = int(N)
	}
	return
}

func (sc *streamDecompressor) consumer() {
	var err error
	for {
		if sc.zr == nil {
			sc.zr, err = zlib.NewReader(sc.pr)
			if err != nil {
				_ = sc.pr.CloseWithError(err)
				return
			}
		}

		_, err := io.Copy(sc.w, sc.zr)
		if err != nil {
			_ = sc.pr.CloseWithError(err)
			return
		}
	}
}

func StreamTest() StreamDecorator {
	sd := StreamDecorator{}
	sd.p =
		func(sp StreamProcessor) StreamProcessor {

			dc := newStreamDecompresser(sp)
			sd.Close = func() error {
				if c, ok := dc.(io.Closer); ok {
					return c.Close()
				}
				return nil
			}

			return StreamProcessWith(dc.Write)
			/*
				return StreamProcessWith(func(p []byte) (n int, err error) {
					var buf bytes.Buffer
					if n, err := io.Copy(w, bytes.NewReader(p)); err != nil {
						return int(n), err
					}
					return sp.Write(buf.Bytes())
				})
			*/

		}
	return sd
}
