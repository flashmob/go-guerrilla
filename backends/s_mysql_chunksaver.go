package backends

import (
	"bytes"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"strconv"
)

func init() {
	streamers["MysqlChunksaver"] = func() *StreamDecorator {
		return MysqlChunksaver()
	}
}

/**
 * messages: mid, part_tree, part_count, has_attach, created_at
 * parts: mid, part_id, chunk_md5, header_data, seq
 * chunk: md5, references, data, created_at
 * A chunk ends ether: after 64KB or after end of a part
 *
 * - buffer first chunk
 * - if didn't receive first chunk for more than x bytes, save normally
 *
 */
func MysqlChunksaver() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {

			var (
				envelope    *mail.Envelope
				currentPart int
				chunkBuffer bytes.Buffer
			)
			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {

				return nil
			}))

			sd.Open = func(e *mail.Envelope) error {
				// create a new entry & grab the id
				envelope = e
				currentPart = 0
				return nil
			}

			sd.Close = func() error {
				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.MimeHeader); ok {
					for _, v := range *parts {
						fmt.Println(v.Part + " " + strconv.Itoa(int(v.StartingPos)) + " " + strconv.Itoa(int(v.StartingPosBody)) + " " + strconv.Itoa(int(v.EndingPosBody)))
					}
				}
				chunkBuffer.Reset()

				// finalize the message

				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				_ = envelope
				if len(envelope.Header) > 0 {

				}
				var parts []*mime.MimeHeader
				if val, ok := envelope.Values["MimeParts"]; !ok {
					//envelope.Values["MimeParts"] = &parser.Parts
					parts = val.([]*mime.MimeHeader)
					size := len(parts)
					if currentPart != size {
						currentPart = size
						// a new part! todo: code to start a new part
						if currentPart == 0 {

						}
					}
				}

				return sp.Write(p)
			})
		}

	return sd
}
