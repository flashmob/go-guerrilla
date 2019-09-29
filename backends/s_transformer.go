package backends

import (
	"bytes"
	"github.com/flashmob/go-guerrilla/chunk/transfer"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"io"
	"regexp"
	"sync"
)

// ----------------------------------------------------------------------------------
// Processor Name: transformer
// ----------------------------------------------------------------------------------
// Description   : Transforms from base64 / q-printable to 8bit and converts charset to utf-8
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         :
// ----------------------------------------------------------------------------------
// Output        : 8bit mime message
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
	parts               *mime.Parts
	partsCachedOriginal *mime.Parts
	envelope            *mail.Envelope

	state   int
	matched int

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
	if parts, ok := t.envelope.Values["MimeParts"].(*mime.Parts); ok {
		_ = parts
		parts = t.partsCachedOriginal
	}
}

func (t *Transform) ReWrite(b []byte) (count int, err error) {
	if !t.isBody {
		count = len(b)
		if i, err := io.Copy(&t.buf, bytes.NewReader(b)); err != nil {
			return int(i), err
		}
		for {
			line, rErr := t.buf.ReadBytes('\n')
			if rErr == nil {
				if bytes.Contains(line, []byte("Content-Transfer-Encoding: base64")) {
					line = bytes.Replace(line, []byte("base64"), []byte("8bit"), 1)
					t.current.TransferEncoding = "8bit"
					t.current.Charset = "utf8"
				} else if bytes.Contains(line, []byte("charset=")) {
					rx := regexp.MustCompile("charset=\".+?\"")
					line = rx.ReplaceAll(line, []byte("charset=\"utf8\""))
				}
				_, err = io.Copy(t.parser, bytes.NewReader(line))
				if err != nil {
					return
				}

			} else {
				break
			}
		}
	} else {

		// do body decode here
		t.pr, t.pw = io.Pipe()
		if t.decoder == nil {
			t.buf.Reset()
			// the decoder will be reading from an underlying pipe
			t.decoder, err = transfer.NewBodyDecoder(t.pr, transfer.Base64, "iso-8859-1")
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
				//charset_pos = 0
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
							//reWriter.recalculate()
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

				return total, err
			})
		}

	return sd
}
