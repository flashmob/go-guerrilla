package backends

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
	"net/textproto"
	"strconv"
)

// ----------------------------------------------------------------------------------
// Name          : Mime Analyzer
// ----------------------------------------------------------------------------------
// Description   : analyse the MIME structure of a stream
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         :
// ----------------------------------------------------------------------------------
// Output        :
// ----------------------------------------------------------------------------------

func init() {
	streamers["mimeanalyzer"] = func() *StreamDecorator {
		return StreamMimeAnalyzer()
	}
}

type mimepart struct {
	/*
			[starting-pos] => 0
		    [starting-pos-body] => 270
		    [ending-pos] => 2651
		    [ending-pos-body] => 2651
		    [line-count] => 72
		    [body-line-count] => 65
		    [charset] => us-ascii
		    [transfer-encoding] => 8bit
		    [content-boundary] => D7F------------D7FD5A0B8AB9C65CCDBFA872
		    [content-type] => multipart/mixed
		    [content-base] => /

			[starting-pos] => 2023
		    [starting-pos-body] => 2172
		    [ending-pos] => 2561
		    [ending-pos-body] => 2561
		    [line-count] => 9
		    [body-line-count] => 5
		    [charset] => us-ascii
		    [transfer-encoding] => base64
		    [content-name] => map_of_Argentina.gif
		    [content-type] => image/gif
		    [disposition-fi1ename] => map_of_Argentina.gif
		    [content-disposition] => in1ine
		    [content-base] => /
	*/
}

const (
	maxBoundaryLen = 70 + 10
	doubleDash     = "--"
)

type parser struct {
	state int

	accept bytes.Buffer

	once            bool
	boundaryMatched int

	// related to the buffer
	buf                         []byte
	pos                         int
	ch                          byte
	gotNewSlice, consumed, halt chan bool
	result                      chan parserMsg
	isHalting                   bool

	// mime variables
	parts   []*mimeHeader
	msgPos  uint
	msgLine uint
}

type mimeHeader struct {
	headers textproto.MIMEHeader

	part string

	startingPos      uint // including header (after boundary, 0 at the top)
	startingPosBody  uint // after header \n\n
	endingPos        uint // redundant (same as endingPos)
	endingPosBody    uint // the char before the boundary marker
	lineCount        uint
	bodyLineCount    uint
	charset          string
	transferEncoding string
	contentBoundary  string
	contentType      *contentType
	contentBase      string

	dispositionFi1eName string
	contentDisposition  string
	contentName         string
}

type contentType struct {
	superType  string
	subType    string
	parameters map[string]string
}

func (c *contentType) String() string {
	return fmt.Sprintf("%s/%s", c.superType, c.subType)
}

func NewMimeHeader() *mimeHeader {
	mh := new(mimeHeader)
	mh.headers = make(textproto.MIMEHeader, 1)
	return mh
}

func (p *parser) addPart(mh *mimeHeader, id string) {
	mh.part = id
	p.parts = append(p.parts, mh)
}

func (p *parser) endBody(mh *mimeHeader) {

}

//
func (p *parser) more() bool {
	p.consumed <- true // signal that we've reached the end of available input
	select {
	// wait for a new new slice
	case gotMore := <-p.gotNewSlice:
		if !gotMore {
			// no more data, closing
			return false
		}
	case <-p.halt:
		p.isHalting = true
		return false
	}
	return true
}

func (p *parser) next() byte {
	// wait for a new new slice if reached the end
	if p.pos+1 >= len(p.buf) {
		if !p.more() {
			p.ch = 0
			return 0
		}
	}

	// go to the next byte
	p.pos++
	p.ch = p.buf[p.pos]
	p.msgPos++
	if p.ch == '\n' {
		p.msgLine++
	}
	return p.ch
}

func (p *parser) peek() byte {

	// reached the end?
	if p.pos+1 >= len(p.buf) {
		if !p.more() {
			p.ch = 0
			return 0
		}
	}

	// peek the next byte
	if p.pos+1 < len(p.buf) {
		return p.buf[p.pos+1]
	}
	return 0
}

// simulate a byte stream
func (p *parser) inject(input ...[]byte) {
	p.set(input[0])
	p.pos = 0
	p.ch = p.buf[0]
	go func() {
		for i := 1; i < len(input); i++ {
			<-p.consumed
			p.set(input[i])
			p.gotNewSlice <- true
		}
		<-p.consumed
		p.gotNewSlice <- false // no more data
	}()
}

func (p *parser) set(input []byte) {
	if p.pos != -1 {
		// rewind
		p.pos = -1
	}
	p.buf = input

}

// boundary scans until next boundary string, returns error if not found
// syntax specified https://tools.ietf.org/html/rfc2046 p21
func (p *parser) boundary(contentBoundary string) (end bool, err error) {
	defer func() {
		if err == nil {
			if p.ch == '\n' {
				p.next()
			}
		}
		// todo: remove this
		//temp := p.buf[p.pos-10:p.pos+45]
		//_ = temp
	}()
	// gensen chosu
	// shotoku chomen sho
	if len(contentBoundary) < 1 {
		err = errors.New("content boundary too short")
	}
	boundary := doubleDash + contentBoundary
	p.boundaryMatched = 0
	for {
		if i := bytes.Index(p.buf[p.pos:], []byte(boundary)); i > -1 {
			// advance the pointer to 1 char before the end of the boundary
			// then let next() to advance the last char.
			// in case the boundary is the tail part of buffer, calling next()
			// will wait until we get a new buffer
			p.pos = p.pos + i + len(boundary) - 1
			p.next()
			end = p.boundaryEnd()
			p.transportPadding()
			if p.ch != '\n' {
				err = errors.New("boundary new line expected")
			}
			return

		} else {
			// search the tail for partial match
			// if one is found, load more data and continue the match
			// if matched, advance buffer in same way as above
			start := len(p.buf) - len(boundary) + 1
			if start < 0 {
				start = 0
			}
			subject := p.buf[start:]

			for i := 0; i < len(subject); i++ {
				if subject[i] == boundary[p.boundaryMatched] {
					p.boundaryMatched++
				} else {
					p.boundaryMatched = 0
				}
			}
			p.pos = len(p.buf) - 1
			p.next() // this will block until new bytes come in
			if p.ch == 0 {
				return false, io.EOF
			} else if p.boundaryMatched > 0 {
				// check for a match by joining the match from the end of the last buf
				// & the beginning of this buf
				if bytes.Compare(
					p.buf[0:len(boundary)-p.boundaryMatched],
					[]byte(boundary[p.boundaryMatched:])) == 0 {
					// advance the pointer
					p.pos += len(boundary) - p.boundaryMatched - 1
					p.next()
					end = p.boundaryEnd()
					p.transportPadding()
					if p.ch != '\n' {
						err = errors.New("boundary new line expected")
					}
					return
				}
				p.boundaryMatched = 0
			}
			_ = subject
		}
	}
}

// is it the end of a boundary?
func (p *parser) boundaryEnd() bool {
	if p.ch == '-' && p.peek() == '-' {
		p.next()
		p.next()
		return true
	}
	return false
}

// *LWSP-char
// = *(WSP / CRLF WSP)
func (p *parser) transportPadding() {
	for {
		if p.ch == ' ' || p.ch == '\t' {
			p.next()
		} else if c := p.peek(); p.ch == '\n' && (c == ' ' || c == '\t') {
			p.next()
			p.next()
		} else {
			return
		}
	}
}

type parserMsg struct {
	err error
}

func (p *parser) engine() {
	var err error

	p.next() // load in some bytes
	for {
		err = p.message()
		p.result <- parserMsg{err}
		//p.next()
		p.next()
		if p.isHalting {
			return
		}
	}

}

func (p *parser) message() error {
	var err error

	if p.isWSP(p.ch) {
		err = errors.New("headers cannot start with w-space")
		return err
	}
	mh := NewMimeHeader()
	if err = p.header(mh); err != nil {
		return err
	}

	if p.ch == '\n' && p.next() == '\n' {
		err = p.body(mh)
	} else {
		err = errors.New("body not found")
	}
	return err
}

func (p *parser) header(mh *mimeHeader) (err error) {
	var state int
	var name string

	defer func() {
		fmt.Println(mh.headers)
		p.accept.Reset()
		if val := mh.headers.Get("Content-Transfer-Encoding"); val != "" {
			mh.transferEncoding = val
		}
		if val := mh.headers.Get("Content-Disposition"); val != "" {
			mh.contentDisposition = val
		}

	}()
	mh.startingPos = p.msgPos
	for {

		switch state {
		case 0:
			if (p.ch >= 33 && p.ch <= 126) && p.ch != ':' {
				// capture
				p.accept.WriteByte(p.ch)
			} else if p.ch == ':' {
				state = 1
			} else if p.ch == ' ' && p.peek() == ':' { // tolerate a SP before the :
				p.next()
				state = 1
			} else {
				pc := p.peek()
				err = errors.New("unexpected char:" + string(p.ch) + string(pc))
				return
			}
			if state == 1 {
				if p.accept.Len() < 2 {
					err = errors.New("header field too short")
					return
				}
				name = p.accept.String()
				p.accept.Reset()
				if c := p.peek(); c == ' ' {
					// skip the space
					p.next()
				}
				p.next()
				continue
			}

		case 1:

			if name == "Content-Type" {
				var err error
				contentType, err := p.contentType()
				if err != nil {
					return err
				}
				mh.contentType = &contentType
				if val, ok := contentType.parameters["boundary"]; ok {
					mh.contentBoundary = val
				}
				if val, ok := contentType.parameters["charset"]; ok {
					mh.charset = val
				}
				if val, ok := contentType.parameters["name"]; ok {
					mh.contentName = val
				}
				mh.headers.Add("Content-Type", contentType.String())
				state = 0
			} else {
				if (p.ch >= 33 && p.ch <= 126) || p.isWSP(p.ch) {
					p.accept.WriteByte(p.ch)
				} else if p.ch == '\n' {
					c := p.peek()

					if p.isWSP(c) {
						break // skip \n
					} else {
						mh.headers.Add(name, p.accept.String())
						p.accept.Reset()

						state = 0
					}
				} else {
					err = errors.New("parse error")
					return
				}
			}

		}
		if p.ch == '\n' && p.peek() == '\n' {
			return nil
		}
		p.next()

		if p.ch == 0 {
			return io.EOF
		}

	}

}

func (p *parser) isWSP(b byte) bool {
	return b == ' ' || b == '\t'
}

// type "/" subtype
// *(";" parameter)

// content disposition
// The Content-Disposition Header Field (rfc2183)
// https://stackoverflow.com/questions/48347574/do-rfc-standards-require-the-filename-value-for-mime-attachment-to-be-encapsulat
func (p *parser) contentDisposition() (result contentType, err error) {
	result = contentType{}
	return
}

func (p *parser) contentType() (result contentType, err error) {
	result = contentType{}

	if result.superType, err = p.mimeType(); err != nil {
		return
	}
	if p.ch != '/' {
		return result, errors.New("missing subtype")
	}
	p.next()

	if result.subType, err = p.mimeSubType(); err != nil {
		return
	}
	if p.ch == ';' {
		p.next()
		for {
			if p.ch == '\n' {
				c := p.peek()
				if p.isWSP(c) {
					p.next() // skip \n (FWS)
					continue
				}
				if c == '\n' { // end of header
					return
				}
			}
			if p.isWSP(p.ch) { // skip WSP
				p.next()
				continue
			}
			if p.ch == '(' {
				if err = p.comment(); err != nil {
					return
				}
				continue
			}

			if p.ch > 32 && p.ch < 128 && !isTokenSpecial[p.ch] {
				if key, val, err := p.parameter(); err != nil {
					return result, err
				} else {
					if result.parameters == nil {
						result.parameters = make(map[string]string, 1)
					}
					result.parameters[key] = val
				}
			} else {
				break
			}

		}
	}

	return
}

var isTokenSpecial = [128]bool{
	'(':  true,
	')':  true,
	'<':  true,
	'>':  true,
	'@':  true,
	',':  true,
	';':  true,
	':':  true,
	'\\': true,
	'"':  true,
	'/':  true,
	'[':  true,
	']':  true,
	'?':  true,
	'=':  true,
}

func (p *parser) mimeType() (str string, err error) {

	defer func() {
		if p.accept.Len() > 0 {
			str = p.accept.String()
			p.accept.Reset()
		}
	}()
	if p.ch < 128 && p.ch > 32 && !isTokenSpecial[p.ch] {
		for {
			p.accept.WriteByte(p.ch)
			p.next()
			if !(p.ch < 128 && p.ch > 32 && !isTokenSpecial[p.ch]) {
				return
			}

		}
	} else {
		err = errors.New("unexpected tok")
		return
	}
}

func (p *parser) mimeSubType() (str string, err error) {
	return p.mimeType()
}

// comment     =  "(" *(ctext / quoted-pair / comment) ")"
//
// ctext       =  <any CHAR excluding "(",     ; => may be folded
//                     ")", "\" & CR, & including
//                     linear-white-space>
//
// quoted-pair =  "\" CHAR                     ; may quote any char
func (p *parser) comment() (err error) {
	// all header fields except for Content-Disposition
	// can include RFC 822 comments
	if p.ch != '(' {
		err = errors.New("unexpected token")
	}

	for {
		p.next()
		if p.ch == ')' {
			p.next()
			return
		}
	}

}

func (p *parser) token() (str string, err error) {
	defer func() {
		if err == nil {
			str = p.accept.String()
		}
		if p.accept.Len() > 0 {
			p.accept.Reset()
		}
	}()
	var once bool // must match at least 1 good char
	for {
		if p.ch > 32 && p.ch < 128 && !isTokenSpecial[p.ch] {
			p.accept.WriteByte(p.ch)
			once = true
		} else if !once {
			err = errors.New("invalid token")
			return
		} else {
			return
		}
		p.next()
	}
}

// quoted-string  = ( <"> *(qdtext | quoted-pair ) <"> )
// quoted-pair    = "\" CHAR
// CHAR           = <any US-ASCII character (octets 0 - 127)>
// qdtext         = <any TEXT except <">>
// TEXT           = <any OCTET except CTLs, but including LWS>
func (p *parser) quotedString() (str string, err error) {
	defer func() {
		if err == nil {
			str = p.accept.String()
		}
		if p.accept.Len() > 0 {
			p.accept.Reset()
		}
	}()

	if p.ch != '"' {
		err = errors.New("unexpected token")
		return
	}
	p.next()
	state := 0
	for {
		switch state {
		case 0: // inside quotes

			if p.ch == '"' {
				p.next()
				return
			}
			if p.ch == '\\' {
				state = 1
				break
			}
			if (p.ch < 127 && p.ch > 32) || p.isWSP(p.ch) {
				p.accept.WriteByte(p.ch)
			} else {
				err = errors.New("unexpected token")
				return
			}
		case 1:
			// escaped (<any US-ASCII character (octets 0 - 127)>)
			if p.ch != 0 && p.ch <= 127 {
				p.accept.WriteByte(p.ch)
				state = 0
			} else {
				err = errors.New("unexpected token")
				return
			}
		}
		p.next()
	}
}

// parameter := attribute "=" value
// attribute := token
// token := 1*<any (US-ASCII) CHAR except SPACE, CTLs, or tspecials>
// value := token / quoted-string
// CTL := %x00-1F / %x7F
// quoted-string : <"> <">
func (p *parser) parameter() (attribute, value string, err error) {
	defer func() {
		p.accept.Reset()
	}()

	if attribute, err = p.token(); err != nil {
		return "", "", err
	}
	if p.ch != '=' {
		return "", "", errors.New("expecting =")
	}
	p.next()
	if p.ch == '"' {
		if value, err = p.quotedString(); err != nil {
			return
		}
		return
	} else {
		if value, err = p.token(); err != nil {
			return
		}
		return
	}
}

func (p *parser) body(mh *mimeHeader) (err error) {
	var body bytes.Buffer

	if mh.contentBoundary != "" {
		if end, err := p.boundary(mh.contentBoundary); err != nil {
			return err
		} else {
			fmt.Println("boundary end:", end)
		}
		mh.endingPosBody = p.msgPos
		return
	} else {
		for {

			p.next()
			if p.ch == 0 {
				return io.EOF
			}
			if p.ch == '\n' && p.peek() == '\n' {
				p.next()
				mh.endingPosBody = p.msgPos
				return
			}

			body.WriteByte(p.ch)

		}
	}

}

func (p *parser) mime(boundary string, depth string) (err error) {
	count := 1
	h := NewMimeHeader()

	if p.ch >= 33 && p.ch <= 126 {
		err = p.header(h)
		if err != nil {
			//				temp := p.buf[p.pos:p.pos+20]
			//				_ = temp
			return err
		}
	} else {
		fmt.Println("empty header")
	}

	if p.ch == '\n' && p.peek() == '\n' {
		p.next()
		p.next()
	}
	if h.contentBoundary != "" {
		boundary = h.contentBoundary

	}

	if end, bErr := p.boundary(boundary); bErr != nil {
		return bErr
	} else if end {
		h.endingPosBody = p.msgPos
		return
	}

	if depth == "1" {
		p.addPart(h, depth)
	} else {
		p.addPart(h, depth+"."+strconv.Itoa(count))
		if h.contentType != nil &&
			(h.contentType.superType == "message" ||
				h.contentType.superType == "multipart") {
			return p.mime(boundary, depth+"."+strconv.Itoa(count))
		} else {
			count++
		}
	}

	for {

		var part mimeHeader

		part = *NewMimeHeader()
		if p.ch >= 33 && p.ch <= 126 {
			err = p.header(&part)
			if err != nil {
				return err
			}
		}
		if p.ch == '\n' && p.peek() == '\n' {
			p.next()
			p.next()
		}

		p.addPart(&part, depth+"."+strconv.Itoa(count))
		if part.contentType != nil &&
			(part.contentType.superType == "message" ||
				part.contentType.superType == "multipart") {
			return p.mime(boundary, depth+"."+strconv.Itoa(count))
		} else {
			if end, bErr := p.boundary(boundary); bErr != nil {
				return bErr
			} else if end {
				part.endingPosBody = p.msgPos
				break
			}
		}

		count++

	}
	return

}

func (p *parser) mime2(boundary string, depth string) (err error) {
	count := 1
	h := NewMimeHeader()

	if p.ch >= 33 && p.ch <= 126 {
		err = p.header(h)
		if err != nil {
			//				temp := p.buf[p.pos:p.pos+20]
			//				_ = temp
			return err
		}
	} else {
		fmt.Println("empty header")
	}

	if depth == "1" {
		p.addPart(h, depth)
	} else {
		p.addPart(h, depth+"."+strconv.Itoa(count))
		if h.contentType != nil &&
			(h.contentType.superType == "message" ||
				h.contentType.superType == "multipart") {
			depth = depth + "." + strconv.Itoa(count)
		}
	}

	if p.ch == '\n' && p.peek() == '\n' {
		p.next()
		p.next()
	}
	if h.contentBoundary != "" {
		boundary = h.contentBoundary

	}

	if end, bErr := p.boundary(boundary); bErr != nil {
		return bErr
	} else if end {
		h.endingPosBody = p.msgPos
		return
	}

	for {

		var part mimeHeader

		part = *NewMimeHeader()
		if p.ch >= 33 && p.ch <= 126 {
			err = p.header(&part)
			if err != nil {
				return err
			}
		}
		if p.ch == '\n' && p.peek() == '\n' {
			p.next()
			p.next()
		}

		p.addPart(&part, depth+"."+strconv.Itoa(count))
		if part.contentType != nil &&
			(part.contentType.superType == "message" ||
				part.contentType.superType == "multipart") {
			return p.mime(boundary, depth+"."+strconv.Itoa(count))
		} else {

			if end, bErr := p.boundary(boundary); bErr != nil {
				return bErr
			} else if end {
				part.endingPosBody = p.msgPos
				break
			}
		}
		count++

	}
	return

}

func (p *parser) mimeMsg(boundary string, depth string) (err error) {
	count := 0
	for {
		count++
		d := depth + "." + strconv.Itoa(count)
		_ = d

		{
			h := NewMimeHeader()

			if p.ch >= 33 && p.ch <= 126 {
				err = p.header(h)
				if err != nil {
					//				temp := p.buf[p.pos:p.pos+20]
					//				_ = temp
					return err
				}
			} else {
				fmt.Println("empty header")
			}
			if p.ch == '\n' && p.peek() == '\n' {
				p.next()
				p.next()
			}

			if h.contentBoundary != "" {
				boundary = h.contentBoundary
				if end, bErr := p.boundary(boundary); bErr != nil {
					return bErr
				} else if end {
					return
				}
				err = p.mimeMsg(boundary, depth+"."+strconv.Itoa(count))
				if err != nil {
					return
				}
			} else {
				// body-part end
				for {
					if end, bErr := p.boundary(boundary); bErr != nil {
						return bErr
					} else if end {
						return
					}

				}

			}

			if p.ch == 0 {
				return
			}

			if h.contentType == nil {
				fmt.Println("nope")
			}

			p.addPart(h, depth+"."+strconv.Itoa(count))
			if h.contentType != nil &&
				(h.contentType.superType == "message" ||
					h.contentType.superType == "multipart") {

				return p.mimeMsg(boundary, depth+"."+strconv.Itoa(count))
			} else if end, bErr := p.boundary(boundary); bErr != nil {
				return bErr
			} else if end {
				return
			}

		}

	}
}

func (p *parser) close() error {

	p.msgPos = 0
	p.msgLine = 0
	p.gotNewSlice <- false // signal to engine() that there's no more data

	r := <-p.result
	return r.err
	//return nil
}

func (p *parser) parse(buf []byte) error {

	if !p.once {
		<-p.consumed
		p.once = true
	}

	p.set(buf)
	p.gotNewSlice <- true // unblock

	// make sure that engine() is blocked or stopped before we return
	select {
	case <-p.consumed: // wait for it to block on p.gotNewSlice
		return nil
	case r := <-p.result:

		return r.err
	}

}

func newMimeParser() *parser {
	p := new(parser)
	p.consumed = make(chan bool)
	p.gotNewSlice = make(chan bool)
	p.halt = make(chan bool)
	p.result = make(chan parserMsg, 1)

	return p
}

func (p *parser) start() {
	go p.engine()
}

func StreamMimeAnalyzer() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {

			var (
				envelope *mail.Envelope
				parseErr error
				parser   *parser
			)
			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
				parser = newMimeParser()
				parser.start()
				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {
				//<-parser.end
				//parser.halt <- true
				return nil
			}))

			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				return nil
			}

			sd.Close = func() error {
				if parseErr != nil {
					return nil
				}
				err := parser.close()
				if err != nil {
					fmt.Println("parse err", err)
				}
				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				_ = envelope
				if len(envelope.Header) > 0 {

				}
				if parseErr == nil {
					parseErr = parser.parse(p)
					if parseErr != nil {
						fmt.Println("parse err", parseErr)
					}
				}

				return sp.Write(p)
			})
		}

	return sd
}
