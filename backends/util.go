package backends

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"fmt"
	"io"
	"net/textproto"
	"regexp"
	"strings"
)

// First capturing group is header name, second is header value.
// Accounts for folding headers.
var headerRegex, _ = regexp.Compile(`^([\S ]+):([\S ]+(?:\r\n\s[\S ]+)?)`)

// ParseHeaders is deprecated, see mail.Envelope.ParseHeaders instead
func ParseHeaders(mailData string) map[string]string {
	var headerSectionEnds int
	for i, char := range mailData[:len(mailData)-4] {
		if char == '\r' {
			if mailData[i+1] == '\n' && mailData[i+2] == '\r' && mailData[i+3] == '\n' {
				headerSectionEnds = i + 2
			}
		}
	}
	headers := make(map[string]string)
	matches := headerRegex.FindAllStringSubmatch(mailData[:headerSectionEnds], -1)
	for _, h := range matches {
		name := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(strings.Replace(h[1], "\r\n", "", -1)))
		val := strings.TrimSpace(strings.Replace(h[2], "\r\n", "", -1))
		headers[name] = val
	}
	return headers
}

// returns an md5 hash as string of hex characters
func MD5Hex(stringArguments ...string) string {
	h := md5.New()
	var r *strings.Reader
	for i := 0; i < len(stringArguments); i++ {
		r = strings.NewReader(stringArguments[i])
		io.Copy(h, r)
	}
	sum := h.Sum([]byte{})
	return fmt.Sprintf("%x", sum)
}

// concatenate & compress all strings  passed in
func Compress(stringArguments ...string) string {
	var b bytes.Buffer
	var r *strings.Reader
	w, _ := zlib.NewWriterLevel(&b, zlib.BestSpeed)
	for i := 0; i < len(stringArguments); i++ {
		r = strings.NewReader(stringArguments[i])
		io.Copy(w, r)
	}
	w.Close()
	return b.String()
}
