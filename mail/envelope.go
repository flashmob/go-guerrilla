package mail

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"time"
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
}

const maxHeaderChunk = 1 + (4 << 10) // 4KB

// Address encodes an email address of the form `<user@host>`
type Address struct {
	// User is local part
	User string
	// Host is the domain
	Host string
	// ADL is at-domain list if matched
	ADL []string
	// PathParams contains any ESTMP parameters that were matched
	PathParams [][]string
	// NullPath is true if <> was received
	NullPath bool
	// Quoted indicates if the local-part needs quotes
	Quoted bool
	// IP stores the IP Address, if the Host is an IP
	IP net.IP
}

func (ep *Address) String() string {
	var local string
	if ep.IsEmpty() {
		return ""
	}
	if ep.Quoted {
		var sb bytes.Buffer
		sb.WriteByte('"')
		for i := 0; i < len(ep.User); i++ {
			if ep.User[i] == '\\' || ep.User[i] == '"' {
				// escape
				sb.WriteByte('\\')
			}
			sb.WriteByte(ep.User[i])
		}
		sb.WriteByte('"')
		local = sb.String()
	} else {
		local = ep.User
	}
	if ep.Host != "" {
		return fmt.Sprintf("%s@%s", local, ep.Host)
	}
	return local
}

func (ep *Address) IsEmpty() bool {
	return ep.User == "" && ep.Host == ""
}

var ap = mail.AddressParser{}

// NewAddress takes a string of an RFC 5322 address of the
// form "Gogh Fir <gf@example.com>" or "foo@example.com".
// TODO its not parsing ip addresses properly
func NewAddress(str string) (*Address, error) {
	var isQuoted, isIP bool
	var pos int
	var a *mail.Address
	var err error
	address := new(Address)
	if pos = strings.LastIndex(str, "@"); pos > 0 && str[pos-1] == '"' {
		isQuoted = true
	}
	if pos > 0 && pos+1 < len(str) && str[pos+1] == '[' {
		isIP = true
	}

	a, err = ap.Parse(str)

	if err != nil {
		return nil, err
	}
	pos = strings.LastIndex(a.Address, "@")

	if pos > 0 {
		address.User = a.Address[0:pos]
		address.Host = a.Address[pos+1:]
		if isQuoted {
			address.Quoted = true
		}
		if isIP {
			// check if the ip address is valid
			if v := net.ParseIP(address.Host); v == nil {
				return nil, errors.New("invalid ip")
			} else {
				address.IP = v
				// this will normalize ipv6 addresses
				address.Host = v.String()
			}
		}
		return address, nil
	}
	return nil, errors.New("invalid address")
}

// Envelope of Email represents a single SMTP message.
type Envelope struct {
	// Remote IP address
	RemoteIP string
	// Message sent in EHLO command
	Helo string
	// Sender
	MailFrom Address
	// Recipients
	RcptTo []Address
	// Data stores the header and message body
	Data bytes.Buffer
	// Subject stores the subject of the email, extracted and decoded after calling ParseHeaders()
	Subject string
	// TLS is true if the email was received using a TLS connection
	TLS bool
	// Header stores the results from ParseHeaders()
	Header textproto.MIMEHeader
	// Values hold the values generated when processing the envelope by the backend
	Values map[string]interface{}
	// Hashes of each email on the rcpt
	Hashes []string
	// additional delivery header that may be added
	DeliveryHeader string
	// Email(s) will be queued with this id
	QueuedId string
	// ESMTP: true if EHLO was used
	ESMTP bool
	// When locked, it means that the envelope is being processed by the backend
	sync.Mutex
}

func NewEnvelope(remoteAddr string, clientID uint64) *Envelope {
	return &Envelope{
		RemoteIP: remoteAddr,
		Values:   make(map[string]interface{}),
		QueuedId: queuedID(clientID),
	}
}

func queuedID(clientID uint64) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(string(time.Now().Unix())+string(clientID))))
}

// ParseHeaders parses the headers into Header field of the Envelope struct.
// Data buffer must be full before calling.
// It assumes that at most 30kb of email data can be a header
// Decoding of encoding to UTF is only done on the Subject, where the result is assigned to the Subject field
func (e *Envelope) ParseHeaders() error {
	var err error
	if e.Header != nil {
		return errors.New("headers already parsed")
	}
	buf := e.Data.Bytes()
	// find where the header ends, assuming that over 30 kb would be max
	if len(buf) > maxHeaderChunk {
		buf = buf[:maxHeaderChunk]
	}

	headerEnd := bytes.Index(buf, []byte{'\n', '\n'}) // the first two new-lines chars are the End Of Header
	if headerEnd > -1 {
		header := buf[0 : headerEnd+2]
		headerReader := textproto.NewReader(bufio.NewReader(bytes.NewBuffer(header)))
		e.Header, err = headerReader.ReadMIMEHeader()
		if err == nil || err == io.EOF {
			// decode the subject
			if subject, ok := e.Header["Subject"]; ok {
				e.Subject = MimeHeaderDecode(subject[0])
			}
		}
	} else {
		err = errors.New("header not found")
	}
	return err
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
	e.Lock()
	// got the lock, it means processing finished
	e.Unlock()

	e.MailFrom = Address{}
	e.RcptTo = []Address{}
	// reset the data buffer, keep it allocated
	e.Data.Reset()

	// todo: these are probably good candidates for buffers / use sync.Pool (after profiling)
	e.Subject = ""
	e.Header = nil
	e.Hashes = make([]string, 0)
	e.DeliveryHeader = ""
	e.Values = make(map[string]interface{})
}

// Reseed is called when used with a new connection, once it's accepted
func (e *Envelope) Reseed(remoteIP string, clientID uint64) {
	e.RemoteIP = remoteIP
	e.QueuedId = queuedID(clientID)
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

// MimeHeaderDecode converts 7 bit encoded mime header strings to UTF-8
func MimeHeaderDecode(str string) string {
	state := 0
	var buf bytes.Buffer
	var out []byte
	for i := 0; i < len(str); i++ {
		switch state {
		case 0:
			if str[i] == '=' {
				buf.WriteByte(str[i])
				state = 1
			} else {
				out = append(out, str[i])
			}
		case 1:
			if str[i] == '?' {
				buf.WriteByte(str[i])
				state = 2
			} else {
				out = append(out, str[i])
				buf.Reset()
				state = 0
			}

		case 2:
			if str[i] == ' ' || str[i] == '\t' {
				d, err := Dec.Decode(buf.String())
				if err == nil {
					out = append(out, []byte(d)...)
				} else {
					out = append(out, buf.Bytes()...)
				}
				if skip := hasEncodedWordAhead(str, i); skip != -1 {
					i = skip
				} else {
					out = append(out, str[i])
				}
				buf.Reset()
				state = 0
			} else {
				buf.WriteByte(str[i])
			}
		}
	}
	if buf.Len() > 0 {
		d, err := Dec.Decode(buf.String())
		if err == nil {
			out = append(out, []byte(d)...)
		} else {
			out = append(out, buf.Bytes()...)
		}
	}
	return string(out)
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

func (p *Pool) Borrow(remoteAddr string, clientID uint64) *Envelope {
	var e *Envelope
	p.sem <- true // block the envelope until more room
	select {
	case e = <-p.pool:
		e.Reseed(remoteAddr, clientID)
	default:
		e = NewEnvelope(remoteAddr, clientID)
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
