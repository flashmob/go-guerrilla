package envelope

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/sloonz/go-qprintable"
	"gopkg.in/iconv.v1"
	"io/ioutil"
	"net/textproto"
	"regexp"
	"strings"
)

// EmailAddress encodes an email address of the form `<user@host>`
type EmailAddress struct {
	User string
	Host string
}

func (ep *EmailAddress) String() string {
	return fmt.Sprintf("%s@%s", ep.User, ep.Host)
}

func (ep *EmailAddress) IsEmpty() bool {
	return ep.User == "" && ep.Host == ""
}

// Email represents a single SMTP message.
type Envelope struct {
	// Remote IP address
	RemoteAddress string
	// Message sent in EHLO command
	Helo string
	// Sender
	MailFrom EmailAddress
	// Recipients
	RcptTo []EmailAddress
	// Data stores the header and message body
	Data bytes.Buffer
	// Subject stores the subject of the email, extracted and decoded after calling ParseHeaders()
	Subject string
	// TLS is true if the email was received using a TLS connection
	TLS bool
	// Header stores the results from ParseHeaders()
	Header textproto.MIMEHeader
	// Hold the information generated when processing the envelope by the backend
	Info map[string]interface{}
	// Hashes of each email on the rcpt
	Hashes []string
	//
	DeliveryHeader string
}

func NewEnvelope(remoteAddr string) *Envelope {
	return &Envelope{
		RemoteAddress: remoteAddr,
		Info:          make(map[string]interface{}),
	}
}

// ParseHeaders parses the headers into Header field of the Envelope struct.
// Data buffer must be full before calling.
// It assumes that at most 30kb of email data can be a header
// Decoding of encoding to UTF is only done on the Subject, where the result is assigned to the Subject field
func (e *Envelope) ParseHeaders() error {
	var err error
	if e.Header != nil {
		return errors.New("Headers already parsed")
	}
	b2 := bytes.NewBuffer(e.Data.Bytes())
	// find where the header ends, assuming that over 30 kb would be max
	max := 1024 * 30
	if b2.Len() < max {
		max = b2.Len()
	}
	// read in the chunk which we'll scan for the header
	chunk := make([]byte, max)
	b2.Read(chunk)
	headerEnd := strings.Index(string(chunk), "\n\n") // the first two new-lines is the end of header
	if headerEnd > -1 {
		header := chunk[0:headerEnd]
		headerReader := textproto.NewReader(bufio.NewReader(bytes.NewBuffer(header)))
		e.Header, err = headerReader.ReadMIMEHeader()
		if err != nil {
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

var mimeRegex, _ = regexp.Compile(`=\?(.+?)\?([QBqp])\?(.+?)\?=`)

// Decode strings in Mime header format
// eg. =?ISO-2022-JP?B?GyRCIVo9dztSOWJAOCVBJWMbKEI=?=
func MimeHeaderDecode(str string) string {

	matched := mimeRegex.FindAllStringSubmatch(str, -1)
	var charset, encoding, payload string
	if matched != nil {
		for i := 0; i < len(matched); i++ {
			if len(matched[i]) > 2 {
				charset = matched[i][1]
				encoding = strings.ToUpper(matched[i][2])
				payload = matched[i][3]
				switch encoding {
				case "B":
					str = strings.Replace(
						str,
						matched[i][0],
						MailTransportDecode(payload, "base64", charset),
						1)
				case "Q":
					str = strings.Replace(
						str,
						matched[i][0],
						MailTransportDecode(payload, "quoted-printable", charset),
						1)
				}
			}
		}
	}
	return str
}

// decode from 7bit to 8bit UTF-8
// encodingType can be "base64" or "quoted-printable"
func MailTransportDecode(str string, encodingType string, charset string) string {
	if charset == "" {
		charset = "UTF-8"
	} else {
		charset = strings.ToUpper(charset)
	}
	if encodingType == "base64" {
		str = fromBase64(str)
	} else if encodingType == "quoted-printable" {
		str = fromQuotedP(str)
	}

	if charset != "UTF-8" {
		charset = fixCharset(charset)
		// iconv is pretty good at what it does
		if cd, err := iconv.Open("UTF-8", charset); err == nil {
			defer func() {
				cd.Close()
				if r := recover(); r != nil {
					//logln(1, fmt.Sprintf("Recovered in %v", r))
				}
			}()
			// eg. charset can be "ISO-2022-JP"
			return cd.ConvString(str)
		}

	}
	return str
}

func fromBase64(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := base64.NewDecoder(base64.StdEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

func fromQuotedP(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := qprintable.NewDecoder(qprintable.BinaryEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

var charsetRegex, _ = regexp.Compile(`[_:.\/\\]`)

func fixCharset(charset string) string {
	fixed_charset := charsetRegex.ReplaceAllString(charset, "-")
	// Fix charset
	// borrowed from http://squirrelmail.svn.sourceforge.net/viewvc/squirrelmail/trunk/squirrelmail/include/languages.php?revision=13765&view=markup
	// OE ks_c_5601_1987 > cp949
	fixed_charset = strings.Replace(fixed_charset, "ks-c-5601-1987", "cp949", -1)
	// Moz x-euc-tw > euc-tw
	fixed_charset = strings.Replace(fixed_charset, "x-euc", "euc", -1)
	// Moz x-windows-949 > cp949
	fixed_charset = strings.Replace(fixed_charset, "x-windows_", "cp", -1)
	// windows-125x and cp125x charsets
	fixed_charset = strings.Replace(fixed_charset, "windows-", "cp", -1)
	// ibm > cp
	fixed_charset = strings.Replace(fixed_charset, "ibm", "cp", -1)
	// iso-8859-8-i -> iso-8859-8
	fixed_charset = strings.Replace(fixed_charset, "iso-8859-8-i", "iso-8859-8", -1)
	if charset != fixed_charset {
		return fixed_charset
	}
	return charset
}
