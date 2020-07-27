package chunk

import (
	"errors"
	"fmt"
	"net"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mimeparse"
)

// ----------------------------------------------------------------------------------
// Processor Name: ChunkSaver
// ----------------------------------------------------------------------------------
// Description   : Takes the stream and saves it in chunks. Chunks are split on the
//               : chunk_size config setting, and also at the end of MIME parts,
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
// Config Options: chunk_size - maximum chunk size, in bytes
// --------------:-------------------------------------------------------------------
// Input         : e.MimeParts Which is of type *mime.Parts, as populated by "mimeanalyzer"
// ----------------------------------------------------------------------------------
// Output        : Messages are saved using the Storage interface
//               : See store_sql.go and store_sql.go as examples
// ----------------------------------------------------------------------------------

func init() {
	backends.Streamers["chunksaver"] = func() *backends.StreamDecorator {
		return Chunksaver()
	}
}

type Config struct {
	// ChunkMaxBytes controls the maximum buffer size for saving
	// 16KB default.
	ChunkMaxBytes int    `json:"chunk_size,omitempty"`
	StorageEngine string `json:"storage_engine,omitempty"`
}

//const chunkMaxBytes = 1024 * 16 // 16Kb is the default, change using chunk_size config setting
/**
*
 * A chunk ends ether:
 * after xKB or after end of a part, or end of header
 *
 * - buffer first chunk
 * - if didn't receive first chunk for more than x bytes, save normally
 *
*/
func Chunksaver() *backends.StreamDecorator {
	var (
		config Config

		envelope    *mail.Envelope
		chunkBuffer *ChunkingBufferMime
		msgPos      uint
		database    Storage
		written     int64

		// just some headers from the first mime-part
		subject string
		to      string
		from    string

		progress int // tracks which mime parts were processed
	)
	sd := &backends.StreamDecorator{}
	sd.Configure = func(cfg backends.ConfigGroup) error {
		err := sd.ExtractConfig(cfg, &config)
		if err != nil {
			return err
		}
		if chunkBuffer == nil {
			chunkBuffer = NewChunkedBytesBufferMime()
		}
		// database could be injected when Decorate is called
		if database == nil {
			// configure storage if none was injected
			if config.StorageEngine == "" {
				return errors.New("storage_engine setting not configured")
			}
			if makerFn, ok := StorageEngines[config.StorageEngine]; ok {
				database = makerFn()
			} else {
				return fmt.Errorf("storage engine does not exist [%s]", config.StorageEngine)
			}
		}
		err = database.Initialize(cfg)
		if err != nil {
			return err
		}
		// configure the chunks buffer
		if config.ChunkMaxBytes > 0 {
			chunkBuffer.CapTo(config.ChunkMaxBytes)
		} else {
			chunkBuffer.CapTo(chunkMaxBytes)
		}
		return nil
	}

	sd.Shutdown = func() error {
		err := database.Shutdown()
		return err
	}

	sd.GetEmail = func(emailID uint64) (backends.SeekPartReader, error) {
		if database == nil {
			return nil, errors.New("database is nil")
		}
		email, err := database.GetEmail(emailID)
		if err != nil {
			return nil, errors.New("email not found")

		}
		r, err := NewChunkedReader(database, email, 0)
		return r, err
	}

	sd.Decorate =
		func(sp backends.StreamProcessor, a ...interface{}) backends.StreamProcessor {
			// optional dependency injection (you can pass your own instance of Storage or ChunkingBufferMime)
			for i := range a {
				if db, ok := a[i].(Storage); ok {
					database = db
				}
				if buff, ok := a[i].(*ChunkingBufferMime); ok {
					chunkBuffer = buff
				}
			}
			if database != nil {
				chunkBuffer.SetDatabase(database)
			}

			var writeTo uint
			var pos int

			sd.Open = func(e *mail.Envelope) error {
				// create a new entry & grab the id
				written = 0
				progress = 0
				var ip net.IPAddr
				if ret := net.ParseIP(e.RemoteIP); ret != nil {
					ip = net.IPAddr{IP: ret}
				}
				mid, err := database.OpenMessage(
					e.MailFrom.String(),
					e.Helo,
					e.RcptTo[0].String(),
					ip,
					e.MailFrom.String(),
					e.Protocol(),
					e.TransportType,
				)
				if err != nil {
					return err
				}
				e.MessageID = mid
				envelope = e
				return nil
			}

			sd.Close = func() (err error) {
				err = chunkBuffer.Flush()
				if err != nil {
					// TODO we could delete the half saved message here
					return err
				}
				defer chunkBuffer.Reset()
				if envelope.MessageID > 0 {
					err = database.CloseMessage(
						envelope.MessageID,
						written,
						&chunkBuffer.Info,
						subject,
						envelope.QueuedId,
						to,
						from,
					)
					if err != nil {
						return err
					}
				}
				return nil
			}

			fillVars := func(parts *mimeparse.Parts, subject, to, from string) (string, string, string) {
				if len(*parts) > 0 {
					if subject == "" {
						if val, ok := (*parts)[0].Headers["Subject"]; ok {
							subject = val[0]
						}
					}
					if to == "" {
						if val, ok := (*parts)[0].Headers["To"]; ok {
							addr, err := mail.NewAddress(val[0])
							if err == nil {
								to = addr.String()
							}
						}
					}
					if from == "" {
						if val, ok := (*parts)[0].Headers["From"]; ok {
							addr, err := mail.NewAddress(val[0])
							if err == nil {
								from = addr.String()
							}
						}
					}
				}
				return subject, to, from
			}

			// end() triggers a buffer flush, at the end of a header or part-boundary
			end := func(part *mimeparse.Part, offset uint, p []byte, start uint) (int, error) {
				var err error
				var count int
				// write out any unwritten bytes
				writeTo = start - offset
				size := uint(len(p))
				if writeTo > size {
					writeTo = size
				}
				if writeTo > 0 {
					count, err = chunkBuffer.Write(p[pos:writeTo])
					written += int64(count)
					pos += count
					if err != nil {
						return count, err
					}
				} else {
					count = 0
				}
				err = chunkBuffer.Flush()
				if err != nil {
					return count, err
				}
				chunkBuffer.CurrentPart(part)
				return count, nil
			}

			return backends.StreamProcessWith(func(p []byte) (count int, err error) {
				pos = 0
				if envelope.MimeParts == nil {
					return count, errors.New("no message headers found")
				} else if len(*envelope.MimeParts) > 0 {
					parts := envelope.MimeParts
					subject, to, from = fillVars(parts, subject, to, from)
					offset := msgPos
					chunkBuffer.CurrentPart((*parts)[0])
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]

						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos >= msgPos {
							count, err = end(part, offset, p, part.StartingPos)
							if err != nil {
								return count, err
							}
							// end of a part here
							//fmt.Println("->N --end of part ---")

							msgPos = part.StartingPos
						}
						// break chunk on header
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {

							count, err = end(part, offset, p, part.StartingPosBody)
							if err != nil {
								return count, err
							}
							// end of a header here
							//fmt.Println("->H --end of header --")
							msgPos += uint(count)
						}
						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p) > pos {
							count, _ = chunkBuffer.Write(p[pos:])
							written += int64(count)
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
				return sp.Write(p)
			})
		}
	return sd
}
