package chunk

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
)

const chunkMaxBytes = 1024 * 16
const hashByteSize = 16

type HashKey [hashByteSize]byte

// Pack takes a slice and copies each byte to HashKey internal representation
func (h *HashKey) Pack(b []byte) {
	if len(b) < hashByteSize {
		return
	}
	copy(h[:], b[0:hashByteSize])
}

// String implements the Stringer interface from fmt.Stringer
func (h HashKey) String() string {
	return base64.RawStdEncoding.EncodeToString(h[0:hashByteSize])
}

// Hex returns the hash, encoded in hexadecimal
func (h HashKey) Hex() string {
	return fmt.Sprintf("%x", h[:])
}

// UnmarshalJSON implements the Unmarshaler interface from encoding/json
func (h *HashKey) UnmarshalJSON(b []byte) error {
	dbuf := make([]byte, base64.RawStdEncoding.DecodedLen(len(b[1:len(b)-1])))
	_, err := base64.RawStdEncoding.Decode(dbuf, b[1:len(b)-1])
	if err != nil {
		return err
	}
	h.Pack(dbuf)
	return nil
}

// MarshalJSON implements the Marshaler interface from encoding/json
// The value is marshaled as a raw base64 to save some bytes
// eg. instead of typically using hex, de17038001170380011703ff01170380 would be represented as 3hcDgAEXA4ABFwP/ARcDgA
func (h *HashKey) MarshalJSON() ([]byte, error) {
	return []byte(`"` + h.String() + `"`), nil
}

// PartsInfo describes the mime-parts contained in the email
type PartsInfo struct {
	Count       uint32        `json:"c"`   // number of parts
	TextPart    int           `json:"tp"`  // index of the main text part to display
	HTMLPart    int           `json:"hp"`  // index of the main html part to display (if any)
	HasAttach   bool          `json:"a"`   // is there an attachment?
	Parts       []ChunkedPart `json:"p"`   // info describing a mime-part
	CBoundaries []string      `json:"cbl"` // content boundaries list
	Err         error         `json:"e"`   // any error encountered (mimeparse.MimeError)
}

var bp sync.Pool // bytes.buffer pool

// ChunkedPart contains header information about a mime-part, including keys pointing to where the data is stored at
type ChunkedPart struct {
	PartId             string    `json:"i"`
	Size               uint      `json:"s"`
	ChunkHash          []HashKey `json:"h"` // sequence of hashes the data is stored at
	ContentType        string    `json:"t"`
	Charset            string    `json:"c"`
	TransferEncoding   string    `json:"e"`
	ContentDisposition string    `json:"d"`
	ContentBoundary    int       `json:"cb"` // index to the CBoundaries list in PartsInfo
}

func NewPartsInfo() *PartsInfo {
	pi := new(PartsInfo)
	bp = sync.Pool{
		// if not available, then create a new one
		New: func() interface{} {
			var b bytes.Buffer
			return &b
		},
	}
	return pi
}

// boundary takes a string and returns the index of the string in the info.CBoundaries slice
func (info *PartsInfo) boundary(cb string) int {
	for i := range info.CBoundaries {
		if info.CBoundaries[i] == cb {
			return i
		}
	}
	info.CBoundaries = append(info.CBoundaries, cb)
	return len(info.CBoundaries) - 1
}

// UnmarshalJSON unmarshals the JSON and decompresses using zlib
func (info *PartsInfo) UnmarshalJSONZlib(b []byte) error {

	r, err := zlib.NewReader(bytes.NewReader(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	all, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	err = json.Unmarshal(all, info)
	if err != nil {
		return err
	}
	return nil
}

// MarshalJSONZlib marshals and compresses the bytes using zlib
func (info *PartsInfo) MarshalJSONZlib() ([]byte, error) {
	if len(info.Parts) == 0 {
		return []byte{}, errors.New("message contained no parts, was mime analyzer")
	}
	buf, err := json.Marshal(info)
	if err != nil {
		return buf, err
	}
	// borrow a buffer form the pool
	compressed := bp.Get().(*bytes.Buffer)
	// put back in the pool
	defer func() {
		compressed.Reset()
		bp.Put(compressed)
	}()

	zlibw, err := zlib.NewWriterLevel(compressed, 9)
	if err != nil {
		return buf, err
	}
	if _, err := zlibw.Write(buf); err != nil {
		return buf, err
	}
	if err := zlibw.Close(); err != nil {
		return buf, err
	}
	return []byte(`"` + compressed.String() + `"`), nil
}
