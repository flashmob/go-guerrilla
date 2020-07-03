package backends

import (
	"bytes"
	"io"
	"regexp"
	"sync"

	"github.com/flashmob/go-guerrilla/chunk/transfer"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mimeparse"
)

// ----------------------------------------------------------------------------------
// Processor Name: transformer
// ----------------------------------------------------------------------------------
// Description   : Transforms from base64 / q-printable to 8bit and converts charset to utf-8
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         : envelope.MimeParts
// ----------------------------------------------------------------------------------
// Output        : 8bit mime message, with charsets decoded to UTF-8
//               : Note that this processor changes the body counts. Therefore, it makes
//               : a new instance of envelope.MimeParts which is then populated
//               : by parsing the new re-written message
// ----------------------------------------------------------------------------------

func init() {
	Streamers["transformer"] = func() *StreamDecorator {
		return Transformer()
	}
}

// Transform stream processor: convert an email to UTF-8
type Transform struct {
	sp                  io.Writer
	isBody              bool // the next bytes to be sent are body?
	buf                 bytes.Buffer
	current             *mimeparse.Part
	decoder             io.Reader
	pr                  *io.PipeReader
	pw                  *io.PipeWriter
	partsCachedOriginal *mimeparse.Parts
	envelope            *mail.Envelope

	// we re-parse the output since the counts have changed
	// parser implements the io.Writer interface, here output will be sent to it and then forwarded to the next processor
	parser *mimeparse.Parser
}

// swap caches the original parts from envelope.MimeParts
// and point them to our parts
func (t *Transform) swap() *mimeparse.Parts {
	parts := t.envelope.MimeParts
	if parts != nil {
		t.partsCachedOriginal = parts
		parts = &t.parser.Parts
		return parts
	}
	return nil
}

// unswap points the parts from MimeParts back to the original ones
func (t *Transform) unswap() {
	if t.envelope.MimeParts != nil {
		t.envelope.MimeParts = t.partsCachedOriginal
	}
}

// regexpCharset captures the charset value
var regexpCharset = regexp.MustCompile("(?i)charset=\"?(.+)\"?") // (?i) is a flag for case-insensitive

func (t *Transform) ReWrite(b []byte, last bool) (count int, err error) {
	defer func() {
		count = len(b)
	}()
	if !t.isBody {
		// Header re-write, how it works
		// we place the partial header's bytes on a buffer from which we can read one line at a time
		// then we match and replace the lines we want, output replaced live.
		// The following re-writes are mde:
		// - base64 => 8bit
		// - supported non-utf8 charset => utf8
		if i, err := io.Copy(&t.buf, bytes.NewReader(b)); err != nil {
			return int(i), err
		}
		var charsetProcessed bool
		charsetFrom := ""
		for {
			line, rErr := t.buf.ReadBytes('\n')
			if rErr == nil {
				if !charsetProcessed {
					// is charsetFrom supported?
					exists := t.current.Headers.Get("content-type")
					if exists != "" {
						charsetProcessed = true
						charsetFrom = t.current.ContentType.Charset()
						if !mail.SupportsCharset(charsetFrom) {
							charsetFrom = ""
						}
					}
				}

				if bytes.Contains(line, []byte("Content-Transfer-Encoding: base64")) {
					line = bytes.Replace(line, []byte("base64"), []byte("8bit"), 1)
				} else if bytes.Contains(line, []byte("charset")) {
					if match := regexpCharset.FindSubmatch(line); match != nil && len(match) > 0 {
						// test if the encoding is supported
						if charsetFrom != "" {
							// it's supported, we can change it to utf8
							line = regexpCharset.ReplaceAll(line, []byte("charset=utf8"))
						}
					}
				}
				_, err = io.Copy(t.parser, bytes.NewReader(line))
				if err != nil {
					return
				}
				if line[0] == '\n' {
					// end of header
					break
				}
			} else {
				return
			}
		}
	} else {

		if ct := t.current.ContentType.Supertype(); ct == "multipart" || ct == "message" {
			_, err = io.Copy(t.parser, bytes.NewReader(b))
			return
		}

		// Body Decode, how it works:
		// First, the decoder is setup, depending on the source encoding type.
		// Next, since the decoder is an io.Reader, we need to use a pipe to connect it.
		// Subsequent calls write to the pipe in a goroutine and the parent-thread copies the result to the output stream
		// The routine stops feeding the decoder data before EndingPosBody, and not decoding anything after, but still
		// outputting the un-decoded remainder.
		// The decoder is destroyed at the end of the body (when last == true)

		t.pr, t.pw = io.Pipe()
		if t.decoder == nil {
			t.buf.Reset()
			// the decoder will be reading from an underlying pipe
			charsetFrom := t.current.ContentType.Charset()
			if charsetFrom == "" {
				charsetFrom = mail.MostCommonCharset
			}

			if mail.SupportsCharset(charsetFrom) {
				t.decoder, err = transfer.NewBodyDecoder(t.pr, transfer.ParseEncoding(t.current.TransferEncoding), charsetFrom)
				if err != nil {
					return
				}
				t.current.Charset = "utf8"
				t.current.TransferEncoding = "8bit"
			}
		}

		wg := sync.WaitGroup{}
		wg.Add(1)

		// out is the slice that will be decoded
		var out []byte
		// remainder will not be decoded. Typically, this contains the boundary maker, and we want to preserve it
		var remainder []byte
		if t.current.EndingPosBody > 0 {
			size := t.current.EndingPosBody - t.current.StartingPosBody - 1 // -1 since we do not want \n
			out = b[:size]
			remainder = b[size:]
		} else {
			// use the entire slice
			out = b
		}
		go func() {
			// stream our slice to the pipe
			defer wg.Done()
			_, pRrr := io.Copy(t.pw, bytes.NewReader(out))
			if pRrr != nil {
				_ = t.pw.CloseWithError(err)
				return
			}
			_ = t.pw.Close()
		}()
		// do the decoding
		var i int64
		i, err = io.Copy(t.parser, t.decoder)
		// wait for the pipe to finish
		wg.Wait()
		_ = t.pr.Close()

		if last {
			t.decoder = nil
		}
		count += int(i)
		if err != nil {
			return
		}
		// flush any remainder
		if len(remainder) > 0 {
			i, err = io.Copy(t.parser, bytes.NewReader(remainder))
			count += int(i)
			if err != nil {
				return
			}
		}
	}
	return count, err
}

func (t *Transform) Reset() {
	t.decoder = nil
}

func Transformer() *StreamDecorator {

	var (
		msgPos   uint
		progress int
	)
	reWriter := Transform{}

	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var (
				envelope *mail.Envelope
				// total is the total number of bytes written
				total int64
				// pos tracks the current position of the output slice
				pos int
				// written is the number of bytes written out in this call
				written int
			)

			if reWriter.sp == nil {
				reWriter.sp = sp
			}

			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				if reWriter.parser == nil {
					reWriter.parser = mimeparse.NewMimeParserWriter(sp)
					reWriter.parser.Open()
				}
				reWriter.envelope = envelope
				return nil
			}

			sd.Close = func() error {
				total = 0
				return reWriter.parser.Close()
			}

			end := func(part *mimeparse.Part, offset uint, p []byte, start uint) (int, error) {
				var err error
				var count int

				count, err = reWriter.ReWrite(p[pos:start-offset], true)

				written += count
				if err != nil {
					return count, err
				}
				reWriter.current = part
				pos += count
				return count, nil
			}

			return StreamProcessWith(func(p []byte) (count int, err error) {
				pos = 0
				written = 0
				parts := envelope.MimeParts
				if parts != nil && len(*parts) > 0 {

					// we are going to change envelope.MimeParts to our own copy with our own counts
					envelope.MimeParts = reWriter.swap()
					defer func() {
						reWriter.unswap()
						total += int64(written)
					}()

					offset := msgPos
					reWriter.current = (*parts)[0]
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]
						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos >= msgPos {
							count, err = end(part, offset, p, part.StartingPos)
							if err != nil {
								break
							}
							msgPos = part.StartingPos
							reWriter.isBody = false

						}
						// break chunk on header (found the body)
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {
							count, err = end(part, offset, p, part.StartingPosBody)
							if err != nil {
								break
							}
							reWriter.isBody = true
							msgPos += uint(count)

						}

						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p)-1 > pos {
							count, err = reWriter.ReWrite(p[pos:], false)

							written += count
							if err != nil {
								break
							}
							pos += count
							msgPos += uint(count)
						}
						// if there's no more data
						if pos >= len(p) {
							break
						}
					}
					if len(*parts) > 2 {
						progress = len(*parts) - 2 // skip to 2nd last part, assume previous parts are already processed
					}
				}
				// note that in this case, ReWrite method will output the stream to further processors down the line
				// here we just return back with the result
				return written, err
			})
		}
	return sd
}
