package mail

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"mime"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/mail/mimeparse"
	"github.com/flashmob/go-guerrilla/mail/smtp"
)

// A WordDecoder decodes MIME headers containing RFC 2047 encoded-words.
// Used by the MimeHeaderDecode function.
// It's exposed public so that an alternative decoder can be set, eg Gnu iconv
// by importing the mail/inconv package.
// Another alternative would be to use https://godoc.org/golang.org/x/text/encoding
var Dec mime.WordDecoder

func init() {
	// use the default decoder, without Gnu inconv. Import the mail/inconv package to use iconv.
	Dec = mime.WordDecoder{}
	// for QueuedID generation
	hasher.h = fnv.New128a()
}

// Address encodes an email address of the form `<user@host>`
type Address struct {
	// User is local part
	User string
	// Host is the domain
	Host string
	// ADL is at-domain list if matched
	ADL []string
	// PathParams contains any ESTMP parameters that were matched
	PathParams []smtp.PathParam
	// NullPath is true if <> was received
	NullPath bool
	// Quoted indicates if the local-part needs quotes
	Quoted bool
	// IP stores the IP Address, if the Host is an IP
	IP net.IP
	// DisplayName is a label before the address (RFC5322)
	DisplayName string
	// DisplayNameQuoted is true when DisplayName was quoted
	DisplayNameQuoted bool
}

func (a *Address) String() string {
	var local string
	if a.IsEmpty() {
		return ""
	}
	if a.User == "postmaster" && a.Host == "" {
		return "postmaster"
	}
	if a.Quoted {
		var sb bytes.Buffer
		sb.WriteByte('"')
		for i := 0; i < len(a.User); i++ {
			if a.User[i] == '\\' || a.User[i] == '"' {
				// escape
				sb.WriteByte('\\')
			}
			sb.WriteByte(a.User[i])
		}
		sb.WriteByte('"')
		local = sb.String()
	} else {
		local = a.User
	}
	if a.Host != "" {
		if a.IP != nil {
			return fmt.Sprintf("%s@[%s]", local, a.Host)
		}
		return fmt.Sprintf("%s@%s", local, a.Host)
	}
	return local
}

func (a *Address) IsEmpty() bool {
	return a.User == "" && a.Host == ""
}

func (a *Address) IsPostmaster() bool {
	if a.User == "postmaster" {
		return true
	}
	return false
}

// NewAddress takes a string of an RFC 5322 address of the
// form "Gogh Fir <gf@example.com>" or "foo@example.com".
func NewAddress(str string) (*Address, error) {

	var ap smtp.RFC5322
	l, err := ap.Address([]byte(str))
	if err != nil {
		return nil, err
	}
	if len(l.List) == 0 {
		return nil, errors.New("no email address matched")
	}
	a := new(Address)
	addr := &l.List[0]
	a.User = addr.LocalPart
	a.Quoted = addr.LocalPartQuoted
	a.Host = addr.Domain
	a.IP = addr.IP
	a.DisplayName = addr.DisplayName
	a.DisplayNameQuoted = addr.DisplayNameQuoted
	a.NullPath = addr.NullPath
	return a, nil
}

type Hash128 [16]byte

func (h Hash128) String() string {
	return fmt.Sprintf("%x", h[:])
}

// FromHex converts the, string must be 32 bytes
func (h *Hash128) FromHex(s string) {
	if len(s) != 32 {
		panic("hex string must be 32 bytes")
	}
	_, _ = hex.Decode(h[:], []byte(s))
}

// Bytes returns the raw bytes
func (h Hash128) Bytes() []byte { return h[:] }

// Envelope of Email represents a single SMTP message.
type Envelope struct {
	// Data stores the header and message body (when using the non-streaming processor)
	Data bytes.Buffer
	// Subject stores the subject of the email, extracted and decoded after calling ParseHeaders()
	Subject string
	// Header stores the results from ParseHeaders()
	Header textproto.MIMEHeader
	// Values hold the values generated when processing the envelope by the backend
	Values map[string]interface{}
	// Hashes of each email on the rcpt
	Hashes []string
	// DeliveryHeader stores additional delivery header that may be added (used by non-streaming processor)
	DeliveryHeader string
	// Size is the length of message, after being written
	Size int64
	// MimeParts contain the information about the mime-parts after they have been parsed
	MimeParts *mimeparse.Parts
	// MimeError contains any error encountered when parsing mime using the mimeanalyzer
	MimeError error
	// MessageID contains the id of the message after it has been written
	MessageID uint64
	// Remote IP address
	RemoteIP string
	// Message sent in EHLO command
	Helo string
	// Sender
	MailFrom Address
	// Recipients
	RcptTo []Address
	// TLS is true if the email was received using a TLS connection
	TLS bool
	// Email(s) will be queued with this id
	QueuedId Hash128
	// TransportType indicates whenever 8BITMIME extension has been signaled
	TransportType smtp.TransportType
	// ESMTP: true if EHLO was used
	ESMTP bool
	// ServerID records the server's index in the configuration
	ServerID int

	// When locked, it means that the envelope is being processed by the backend
	sync.WaitGroup
}

type queuedIDGenerator struct {
	h hash.Hash
	n [24]byte
	sync.Mutex
}

var hasher queuedIDGenerator

func NewEnvelope(remoteAddr string, clientID uint64, serverID int) *Envelope {
	return &Envelope{
		RemoteIP: remoteAddr,
		Values:   make(map[string]interface{}),
		ServerID: serverID,
		QueuedId: QueuedID(clientID, serverID),
	}
}

func QueuedID(clientID uint64, serverID int) Hash128 {
	hasher.Lock()
	defer func() {
		hasher.h.Reset()
		hasher.Unlock()
	}()
	h := Hash128{}
	// pack the seeds and hash'em
	binary.BigEndian.PutUint64(hasher.n[0:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(hasher.n[8:16], clientID)
	binary.BigEndian.PutUint64(hasher.n[16:24], uint64(serverID))
	hasher.h.Write(hasher.n[:])
	copy(h[:], hasher.h.Sum([]byte{}))
	return h
}

// ParseHeaders parses the headers into Header field of the Envelope struct.
// Data buffer must be full before calling.
// It assumes that at most 30kb of email data can be a header
// Decoding of encoding to UTF is only done on the Subject, where the result is assigned to the Subject field
func (e *Envelope) ParseHeaders() error {
	if e.Header == nil {
		return errors.New("headers not parsed")
	}
	if len(e.Header) == 0 {
		return errors.New("header not found")
	}
	// decode the subject
	if subject, ok := e.Header["Subject"]; ok {
		e.Subject = MimeHeaderDecode(subject[0])
	}
	return nil
}

// Len returns the number of bytes that would be in the reader returned by NewReader()
func (e *Envelope) Len() int {
	return len(e.DeliveryHeader) + e.Data.Len()
}

// NewReader returns a new reader for reading the email contents, including the delivery headers
func (e *Envelope) NewReader() io.Reader {
	return io.MultiReader(
		strings.NewReader(e.DeliveryHeader),
		bytes.NewReader(e.Data.Bytes()),
	)
}

// String converts the email to string.
// Typically, you would want to use the compressor guerrilla.Processor for more efficiency, or use NewReader
func (e *Envelope) String() string {
	return e.DeliveryHeader + e.Data.String()
}

// ResetTransaction is called when the transaction is reset (keeping the connection open)
func (e *Envelope) ResetTransaction() {

	// ensure not processing by the backend, will only get lock if finished, otherwise block
	e.Wait()

	e.MailFrom = Address{}
	e.RcptTo = []Address{}
	// reset the data buffer, keep it allocated
	e.Data.Reset()

	// todo: these are probably good candidates for buffers / use sync.Pool (after profiling)
	e.Subject = ""
	e.Header = nil
	e.Hashes = make([]string, 0)
	e.DeliveryHeader = ""
	e.Size = 0
	e.MessageID = 0
	e.MimeParts = nil
	e.MimeError = nil
	e.Values = make(map[string]interface{})
}

// Reseed is called when used with a new connection, once it's accepted
func (e *Envelope) Reseed(remoteIP string, clientID uint64, serverID int) {
	e.RemoteIP = remoteIP
	e.ServerID = serverID
	e.QueuedId = QueuedID(clientID, serverID)
	e.Helo = ""
	e.TLS = false
	e.ESMTP = false
}

// PushRcpt adds a recipient email address to the envelope
func (e *Envelope) PushRcpt(addr Address) {
	e.RcptTo = append(e.RcptTo, addr)
}

// PopRcpt removes the last email address that was pushed to the envelope
func (e *Envelope) PopRcpt() Address {
	ret := e.RcptTo[len(e.RcptTo)-1]
	e.RcptTo = e.RcptTo[:len(e.RcptTo)-1]
	return ret
}

func (e *Envelope) Protocol() Protocol {
	protocol := ProtocolSMTP
	switch {
	case !e.ESMTP && !e.TLS:
		protocol = ProtocolSMTP
	case !e.ESMTP && e.TLS:
		protocol = ProtocolSMTPS
	case e.ESMTP && !e.TLS:
		protocol = ProtocolESMTP
	case e.ESMTP && e.TLS:
		protocol = ProtocolESMTPS
	}
	return protocol
}

type Protocol int

const (
	ProtocolSMTP Protocol = iota
	ProtocolSMTPS
	ProtocolESMTP
	ProtocolESMTPS
	ProtocolLTPS
	ProtocolUnknown
)

func (p Protocol) String() string {
	switch p {
	case ProtocolSMTP:
		return "SMTP"
	case ProtocolSMTPS:
		return "SMTPS"
	case ProtocolESMTP:
		return "ESMTP"
	case ProtocolESMTPS:
		return "ESMTPS"
	case ProtocolLTPS:
		return "LTPS"
	}
	return "unknown"
}

func ParseProtocolType(str string) Protocol {
	switch {
	case str == "SMTP":
		return ProtocolSMTP
	case str == "SMTPS":
		return ProtocolSMTPS
	case str == "ESMTP":
		return ProtocolESMTP
	case str == "ESMTPS":
		return ProtocolESMTPS
	case str == "LTPS":
		return ProtocolLTPS
	}

	return ProtocolUnknown
}

const (
	statePlainText = iota
	stateStartEncodedWord
	stateEncoding
	stateCharset
	statePayload
	statePayloadEnd
)

// MimeHeaderDecode converts 7 bit encoded mime header strings to UTF-8
func MimeHeaderDecode(str string) string {
	// optimized to only create an output buffer if there's need to
	// the `out` buffer is only made if an encoded word was decoded without error
	// `out` is made with the capacity of len(str)
	// a simple state machine is used to detect the start & end of encoded word and plain-text
	state := statePlainText
	var (
		out        []byte
		wordStart  int  // start of an encoded word
		wordLen    int  // end of an encoded
		ptextStart = -1 // start of plan-text
		ptextLen   int  // end of plain-text
	)
	for i := 0; i < len(str); i++ {
		switch state {
		case statePlainText:
			if ptextStart == -1 {
				ptextStart = i
			}
			if str[i] == '=' {
				state = stateStartEncodedWord
				wordStart = i
				wordLen = 1
			} else {
				ptextLen++
			}
		case stateStartEncodedWord:
			if str[i] == '?' {
				wordLen++
				state = stateCharset
			} else {
				wordLen = 0
				state = statePlainText
				ptextLen++
			}
		case stateCharset:
			if str[i] == '?' {
				wordLen++
				state = stateEncoding
			} else if str[i] >= 'a' && str[i] <= 'z' ||
				str[i] >= 'A' && str[i] <= 'Z' ||
				str[i] >= '0' && str[i] <= '9' || str[i] == '-' {
				wordLen++
			} else {
				// error
				state = statePlainText
				ptextLen += wordLen
				wordLen = 0
			}
		case stateEncoding:
			if str[i] == '?' {
				wordLen++
				state = statePayload
			} else if str[i] == 'Q' || str[i] == 'q' || str[i] == 'b' || str[i] == 'B' {
				wordLen++
			} else {
				// abort
				state = statePlainText
				ptextLen += wordLen
				wordLen = 0
			}

		case statePayload:
			if str[i] == '?' {
				wordLen++
				state = statePayloadEnd
			} else {
				wordLen++
			}

		case statePayloadEnd:
			if str[i] == '=' {
				wordLen++
				var err error
				out, err = decodeWordAppend(ptextLen, out, str, ptextStart, wordStart, wordLen)
				if err != nil && out == nil {
					// special case: there was an error with decoding and `out` wasn't created
					// we can assume the encoded word as plaintext
					ptextLen += wordLen //+ 1 // add 1 for the space/tab
					wordLen = 0
					wordStart = 0
					state = statePlainText
					continue
				}
				if skip := hasEncodedWordAhead(str, i+1); skip != -1 {
					i = skip
				} else {
					out = makeAppend(out, len(str), []byte{})
				}
				ptextStart = -1
				ptextLen = 0
				wordLen = 0
				wordStart = 0
				state = statePlainText
			} else {
				// abort
				state = statePlainText
				ptextLen += wordLen
				wordLen = 0
			}

		}
	}

	if out != nil && ptextLen > 0 {
		out = makeAppend(out, len(str), []byte(str[ptextStart:ptextStart+ptextLen]))
		ptextLen = 0
	}

	if out == nil {
		// best case: there was nothing to encode
		return str
	}
	return string(out)
}

func decodeWordAppend(ptextLen int, out []byte, str string, ptextStart int, wordStart int, wordLen int) ([]byte, error) {
	if ptextLen > 0 {
		out = makeAppend(out, len(str), []byte(str[ptextStart:ptextStart+ptextLen]))
	}
	d, err := Dec.Decode(str[wordStart : wordLen+wordStart])
	if err == nil {
		out = makeAppend(out, len(str), []byte(d))
	} else if out != nil {
		out = makeAppend(out, len(str), []byte(str[wordStart:wordLen+wordStart]))
	}
	return out, err
}

func makeAppend(out []byte, size int, in []byte) []byte {
	if out == nil {
		out = make([]byte, 0, size)
	}
	out = append(out, in...)
	return out
}

func hasEncodedWordAhead(str string, i int) int {
	for ; i+2 < len(str); i++ {
		if str[i] != ' ' && str[i] != '\t' {
			return -1
		}
		if str[i+1] == '=' && str[i+2] == '?' {
			return i
		}
	}
	return -1
}

// Envelopes have their own pool

type Pool struct {
	// envelopes that are ready to be borrowed
	pool chan *Envelope
	// semaphore to control number of maximum borrowed envelopes
	sem chan bool
}

func NewPool(poolSize int) *Pool {
	return &Pool{
		pool: make(chan *Envelope, poolSize),
		sem:  make(chan bool, poolSize),
	}
}

func (p *Pool) Borrow(remoteAddr string, clientID uint64, serverID int) *Envelope {
	var e *Envelope
	p.sem <- true // block the envelope until more room
	select {
	case e = <-p.pool:
		e.Reseed(remoteAddr, clientID, serverID)
	default:
		e = NewEnvelope(remoteAddr, clientID, serverID)
	}
	return e
}

// Return returns an envelope back to the envelope pool
// Make sure that envelope finished processing before calling this
func (p *Pool) Return(e *Envelope) {
	select {
	case p.pool <- e:
		//placed envelope back in pool
	default:
		// pool is full, discard it
	}
	// take a value off the semaphore to make room for more envelopes
	<-p.sem
}

const MostCommonCharset = "ISO-8859-1"

var supportedEncodingsCharsets map[string]bool

func SupportsCharset(charset string) bool {
	if supportedEncodingsCharsets == nil {
		supportedEncodingsCharsets = make(map[string]bool)
	} else if ok, result := supportedEncodingsCharsets[charset]; ok {
		return result
	}
	_, err := Dec.CharsetReader(charset, bytes.NewReader([]byte{}))
	if err != nil {
		supportedEncodingsCharsets[charset] = false
		return false
	}
	supportedEncodingsCharsets[charset] = true
	return true
}
