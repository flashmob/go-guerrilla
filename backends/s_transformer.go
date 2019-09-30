package backends

import (
	"bytes"
	"github.com/flashmob/go-guerrilla/chunk/transfer"
	"io"
	"regexp"
	"sync"

	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
)

// ----------------------------------------------------------------------------------
// Processor Name: transformer
// ----------------------------------------------------------------------------------
// Description   : Transforms from base64 / q-printable to 8bit and converts charset to utf-8
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         : envelope.Values["MimeParts"]
// ----------------------------------------------------------------------------------
// Output        : 8bit mime message, with charsets decoded to UTF-8
//               : Note that this processor changes the body counts. Therefore, it makes
//               : a new instance of envelope.Values["MimeParts"] which is then populated
//               : by parsing the new re-written message
// ----------------------------------------------------------------------------------

func init() {
	Streamers["transformer"] = func() *StreamDecorator {
		return Transformer()
	}
}

type TransformerConfig struct {
	// we can add any config here

}

type Transform struct {
	sp                  io.Writer
	isBody              bool // the next bytes to be sent are body?
	buf                 bytes.Buffer
	current             *mime.Part
	decoder             io.Reader
	pr                  *io.PipeReader
	pw                  *io.PipeWriter
	partsCachedOriginal *mime.Parts
	envelope            *mail.Envelope

	// we re-parse the output since the counts have changed
	// parser implements the io.Writer interface, here output will be sent to it and then forwarded to the next processor
	parser *mime.Parser
}

// cache the original parts from envelope.Values
// and point them to our parts
func (t *Transform) swap() *mime.Parts {

	if parts, ok := t.envelope.Values["MimeParts"].(*mime.Parts); ok {
		t.partsCachedOriginal = parts
		parts = &t.parser.Parts
		return parts
	}
	return nil

}

// point the parts from envelope.Values back to the original ones
func (t *Transform) unswap() {
	if _, ok := t.envelope.Values["MimeParts"].(*mime.Parts); ok {
		t.envelope.Values["MimeParts"] = t.partsCachedOriginal
	}
}

var regexpCharset = regexp.MustCompile("(?i)charset=\"?(.+)\"?") // (?i) is a flag for case-insensitive

// todo: we may optimize this by looking at t.partsCachedOriginal, implement a Reader for it, re-write the header as we read from it

func (t *Transform) ReWrite(b []byte) (count int, err error) {
	if !t.isBody {
		// we place the partial header's bytes on a buffer from which we can read one line at a time
		// then we match and replace the lines we want
		count = len(b)
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
					t.current.TransferEncoding = "8bit"

				} else if bytes.Contains(line, []byte("charset")) {
					if match := regexpCharset.FindSubmatch(line); match != nil && len(match) > 0 {
						// test if the encoding is supported
						if charsetFrom != "" {
							// it's supported, we can change it to utf8
							line = regexpCharset.ReplaceAll(line, []byte("charset=utf8"))
							t.current.Charset = "utf8"
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
				// returned data does not end in delim
				panic("returned data does not end in delim")
				//break
			}
		}
	} else {

		// do body decode here
		t.pr, t.pw = io.Pipe()
		if t.decoder == nil {
			t.buf.Reset()
			// the decoder will be reading from an underlying pipe
			charsetFrom := t.current.ContentType.Charset()
			if charsetFrom == "" {
				charsetFrom = mail.MostCommonCharset
			}
			if mail.SupportsCharset(charsetFrom) {
				t.decoder, err = transfer.NewBodyDecoder(t.pr, transfer.Base64, charsetFrom)
			}
			if err != nil {
				return
			}

		}

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			// stream our slice to the pipe
			defer wg.Done()
			_, pRrr := io.Copy(t.pw, bytes.NewReader(b))
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
		_ = i
		wg.Wait()
		_ = t.pr.Close()
		count = len(b)
	}
	return count, err
}

func (t *Transform) Reset() {
	t.decoder = nil

}

func Transformer() *StreamDecorator {

	var conf *TransformerConfig

	Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&HeaderConfig{})
		bcfg, err := Svc.ExtractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		conf = bcfg.(*TransformerConfig)
		_ = conf
		return nil
	}))

	var msgPos uint
	var progress int
	reWriter := Transform{}

	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var envelope *mail.Envelope
			if reWriter.sp == nil {
				reWriter.sp = sp
			}

			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				_ = envelope
				if reWriter.parser == nil {
					reWriter.parser = mime.NewMimeParserWriter(sp)
					reWriter.parser.Open()
				}
				reWriter.envelope = envelope
				return nil
			}

			return StreamProcessWith(func(p []byte) (count int, err error) {
				var total int
				if parts, ok := envelope.Values["MimeParts"].(*mime.Parts); ok && len(*parts) > 0 {

					// we are going to change envelope.Values["MimeParts"] to our own copy with our own counts
					envelope.Values["MimeParts"] = reWriter.swap()
					defer reWriter.unswap()
					var pos int

					offset := msgPos
					reWriter.current = (*parts)[0]
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]

						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos > msgPos {
							reWriter.isBody = false
							count, err = reWriter.ReWrite(p[pos : part.StartingPos-offset])

							total += count
							if err != nil {
								break
							}
							reWriter.current = part
							pos += count
							msgPos = part.StartingPos
						}
						// break chunk on header (found the body)
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {
							count, err = reWriter.ReWrite(p[pos : part.StartingPosBody-offset])
							total += count
							if err != nil {
								break
							}
							_, _ = reWriter.parser.Write([]byte{'\n'}) // send an end of header to the parser
							reWriter.isBody = true
							reWriter.current = part
							pos += count
							msgPos = part.StartingPosBody
						}
						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p)-1 > pos {
							count, err = reWriter.ReWrite(p[pos:])
							total += count
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
				return total, err
			})
		}
	return sd
}
