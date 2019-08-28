package backends

// ----------------------------------------------------------------------------------
// Processor Name: ChunkSaver
// ----------------------------------------------------------------------------------
// Description   : Takes the stream and saves it in chunks. Chunks are split on the
//               : chunksaver_chunk_size config setting, and also at the end of MIME parts,
//               : and after a header. This allows for basic de-duplication: we can take a
//               : hash of each chunk, then check the database to see if we have it already.
//               : We don't need to write it to the database, but take the reference of the
//               : previously saved chunk and only increment the reference count.
//               : The rationale to put headers and bodies into separate chunks is
//               : due to headers often containing more unique data, while the bodies are
//               : often duplicated, especially for messages that are CC'd or forwarded
// ----------------------------------------------------------------------------------
// Requires      : "mimeanalyzer" stream processor to be enabled before it
// ----------------------------------------------------------------------------------
// Config Options: chunksaver_chunk_size - maximum chunk size, in bytes
// --------------:-------------------------------------------------------------------
// Input         : e.Values["MimeParts"] Which is of type *[]*mime.Part, as populated by "mimeanalyzer"
// ----------------------------------------------------------------------------------
// Output        :
// ----------------------------------------------------------------------------------

import (
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
)

type chunkSaverConfig struct {
	// ChunkMaxBytes controls the maximum buffer size for saving
	// 16KB default. The smallest possible size is 64 bytes to to bytes.Buffer limitation
	ChunkMaxBytes int `json:"chunksaver_chunk_size"`
}

func init() {
	streamers["chunksaver"] = func() *StreamDecorator {
		return Chunksaver()
	}
}

type partsInfo struct {
	Count     uint32 // number of parts
	TextPart  int    // id of the main text part to display
	HTMLPart  int    // id of the main html part to display (if any)
	HasAttach bool
	Parts     []chunkedParts
}

type chunkedParts struct {
	PartId             string
	ChunkHash          [][32]byte // sequence of hashes the data is stored at
	ContentType        string
	Charset            string
	TransferEncoding   string
	ContentDisposition string
}

type chunkedBytesBuffer struct {
	buf []byte
}

// flush signals that it's time to write the buffer out to disk
func (c *chunkedBytesBuffer) flush() {
	fmt.Print(string(c.buf))
	c.Reset()
}

// Reset sets the length back to 0, making it re-usable
func (c *chunkedBytesBuffer) Reset() {
	c.buf = c.buf[:0] // set the length back to 0
}

// Write takes a p slice of bytes and writes it to the buffer.
// It will never grow the buffer, flushing it as
// soon as it's full. It will also flush it after the entire slice is written
func (c *chunkedBytesBuffer) Write(p []byte) (i int, err error) {
	remaining := len(p)
	bufCap := cap(c.buf)
	for {
		free := bufCap - len(c.buf)
		if free > remaining {
			// enough of room in the buffer
			c.buf = append(c.buf, p[i:i+remaining]...)
			i += remaining
			c.flush()
			return
		} else {
			// fill the buffer to the 'brim' with a slice from p
			c.buf = append(c.buf, p[i:i+bufCap]...)
			remaining -= bufCap
			i += bufCap
			c.flush()
		}
	}
}

// CapTo caps the internal buffer to specified number of bytes, sets the length back to 0
func (c *chunkedBytesBuffer) CapTo(n int) {
	if cap(c.buf) == n {
		return
	}
	c.buf = make([]byte, 0, n)
}

const chunkMaxBytes = 1024 * 16 // 16Kb is the default, change using chunksaver_chunk_size config setting
/**
*
 * A chunk ends ether:
 * after xKB or after end of a part, or end of header
 *
 * - buffer first chunk
 * - if didn't receive first chunk for more than x bytes, save normally
 *
*/
func Chunksaver() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {
			var (
				envelope    *mail.Envelope
				chunkBuffer chunkedBytesBuffer
				info        partsInfo
				msgPos      uint
			)

			var config *chunkSaverConfig

			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
				configType := BaseConfig(&chunkSaverConfig{})
				bcfg, err := Svc.ExtractConfig(backendConfig, configType)
				if err != nil {
					return err
				}
				config = bcfg.(*chunkSaverConfig)
				if config.ChunkMaxBytes > 0 {
					chunkBuffer.CapTo(config.ChunkMaxBytes)
				} else {
					chunkBuffer.CapTo(chunkMaxBytes)
				}
				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {
				return nil
			}))

			sd.Open = func(e *mail.Envelope) error {
				// create a new entry & grab the id
				envelope = e
				info = partsInfo{Parts: make([]chunkedParts, 0, 3)}
				_ = info.Count
				return nil
			}

			sd.Close = func() error {
				chunkBuffer.Reset()
				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {

				if envelope.Values == nil {
					return 0, errors.New("no message headers found")
				}

				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.Part); ok {
					var pos int
					offset := msgPos
					for i := range *parts {
						part := (*parts)[i]

						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos > msgPos {
							count, _ := chunkBuffer.Write(p[pos : part.StartingPos-offset])
							pos += count
							msgPos = part.StartingPos
						}

						// break chunk on header
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {
							count, _ := chunkBuffer.Write(p[pos : part.StartingPosBody-offset])
							chunkBuffer.flush()
							pos += count
							msgPos = part.StartingPosBody
						}

						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p)-1 > pos {
							count, _ := chunkBuffer.Write(p[pos:])
							pos += count
							msgPos += uint(count)
						}

						// break out if there's no more data to write out
						if pos >= len(p) {
							break
						}
					}
				}
				return sp.Write(p)
			})

		}

	return sd
}
