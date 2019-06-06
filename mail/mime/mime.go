package mime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// todo
// - content-disposition
// - make the error available

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
	startPos       = -1
)

var NotMime = errors.New("not Mime")

type Parser struct {

	// related to the state of the parser

	buf                   []byte
	pos                   int
	ch                    byte
	gotNewSlice, consumed chan bool
	accept                bytes.Buffer
	boundaryMatched       int
	count                 uint
	result                chan parserMsg
	sync.Mutex
	// mime variables

	// Parts is the mime parts tree. The parser builds the parts as it consumes the input
	// In order to represent the tree in an array, we use Parts.part to store the name of
	// each node. The name of the node is the *path* of the node. The root node is always
	// "1". The child would be "1.1", the next sibling would be "1.2", while the child of
	// "1.2" would be "1.2.1"
	Parts           []*Part
	msgPos          uint
	msgLine         uint
	lastBoundaryPos uint
}

type Part struct {
	Headers textproto.MIMEHeader

	Part string

	StartingPos      uint // including header (after boundary, 0 at the top)
	StartingPosBody  uint // after header \n\n
	EndingPos        uint // redundant (same as endingPos)
	EndingPosBody    uint // the char before the boundary marker
	LineCount        uint
	BodyLineCount    uint
	Charset          string
	TransferEncoding string
	ContentBoundary  string
	ContentType      *contentType
	ContentBase      string

	DispositionFi1eName string
	ContentDisposition  string
	ContentName         string
}

type contentType struct {
	superType  string
	subType    string
	parameters map[string]string
}

type parserMsg struct {
	err error
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

func (c *contentType) String() string {
	return fmt.Sprintf("%s/%s", c.superType, c.subType)
}

func newPart() *Part {
	mh := new(Part)
	mh.Headers = make(textproto.MIMEHeader, 1)
	return mh
}

func (p *Parser) addPart(mh *Part, id string) {
	mh.Part = id
	p.Parts = append(p.Parts, mh)
}

// more waits for more input, returns false if there is no more
func (p *Parser) more() bool {
	p.consumed <- true // signal that we've reached the end of available input
	gotMore := <-p.gotNewSlice
	return gotMore
}

// next reads the next byte and advances the pointer
// returns 0 if no more input can be read
// blocks if at the end of the buffer
func (p *Parser) next() byte {
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

// peek does not advance the pointer, but will block if there's no more
// input in the buffer
func (p *Parser) peek() byte {

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

// inject is used for testing, to simulate a byte stream
func (p *Parser) inject(input ...[]byte) {
	p.msgPos = 0
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

func (p *Parser) set(input []byte) {
	if p.pos != startPos {
		// rewind
		p.pos = startPos
	}
	p.buf = input
}

func (p *Parser) skip(nBytes int) {

	for i := 0; i < nBytes; i++ {
		p.next()
		if p.ch == 0 {
			return
		}
	}
}

// boundary scans until next boundary string, returns error if not found
// syntax specified https://tools.ietf.org/html/rfc2046 p21
func (p *Parser) boundary(contentBoundary string) (end bool, err error) {
	defer func() {
		if err == nil {
			if p.ch == '\n' {
				p.next()
			}
		}
	}()

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

			p.skip(i)
			p.lastBoundaryPos = p.msgPos - 1 // - uint(len(boundary))
			p.skip(len(boundary))
			if end, err = p.boundaryEnd(); err != nil {
				return
			}
			if err = p.transportPadding(); err != nil {
				return
			}
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
			p.skip(len(p.buf))
			if p.ch == 0 {
				return false, io.EOF
			} else if p.boundaryMatched > 0 {
				// check for a match by joining the match from the end of the last buf
				// & the beginning of this buf
				if bytes.Compare(
					p.buf[0:len(boundary)-p.boundaryMatched],
					[]byte(boundary[p.boundaryMatched:])) == 0 {

					// advance the pointer
					p.skip(len(boundary) - p.boundaryMatched)

					p.lastBoundaryPos = p.msgPos - uint(len(boundary)) - 1
					end, err = p.boundaryEnd()
					if err != nil {
						return
					}
					if err = p.transportPadding(); err != nil {
						return
					}
					if p.ch != '\n' {
						err = errors.New("boundary new line expected")
					}
					return
				}
				p.boundaryMatched = 0
			}
			//_ = subject
		}
	}
}

// is it the end of a boundary?
func (p *Parser) boundaryEnd() (result bool, err error) {
	if p.ch == '-' && p.peek() == '-' {
		p.next()
		p.next()
		result = true
	}
	if p.ch == 0 {
		err = io.EOF
	}
	return
}

// *LWSP-char
// = *(WSP / CRLF WSP)
func (p *Parser) transportPadding() (err error) {
	for {
		if p.ch == ' ' || p.ch == '\t' {
			p.next()
		} else if c := p.peek(); p.ch == '\n' && (c == ' ' || c == '\t') {
			p.next()
			p.next()
		} else {
			if p.ch == 0 {
				err = io.EOF
			}
			return
		}
	}
}

func (p *Parser) header(mh *Part) (err error) {
	var state int
	var name string

	defer func() {
		p.accept.Reset()
		if val := mh.Headers.Get("Content-Transfer-Encoding"); val != "" {
			mh.TransferEncoding = val
		}
		if val := mh.Headers.Get("Content-Disposition"); val != "" {
			mh.ContentDisposition = val
		}

	}()

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
				err = errors.New("unexpected char:[" + string(p.ch) + "], peek:" +
					string(pc) + ", pos:" + strconv.Itoa(int(p.msgPos)))
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
				mh.ContentType = &contentType
				if val, ok := contentType.parameters["boundary"]; ok {
					mh.ContentBoundary = val
				}
				if val, ok := contentType.parameters["charset"]; ok {
					mh.Charset = val
				}
				if val, ok := contentType.parameters["name"]; ok {
					mh.ContentName = val
				}
				mh.Headers.Add("Content-Type", contentType.String())
				state = 0
			} else {
				if (p.ch >= 33 && p.ch <= 126) || p.isWSP(p.ch) {
					p.accept.WriteByte(p.ch)
				} else if p.ch == '\n' {
					c := p.peek()

					if p.isWSP(c) {
						break // skip \n
					} else {
						mh.Headers.Add(name, p.accept.String())
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

func (p *Parser) isWSP(b byte) bool {
	return b == ' ' || b == '\t'
}

// type "/" subtype
// *(";" parameter)

// content disposition
// The Content-Disposition Header Field (rfc2183)
// https://stackoverflow.com/questions/48347574/do-rfc-standards-require-the-filename-value-for-mime-attachment-to-be-encapsulat
func (p *Parser) contentDisposition() (result contentType, err error) {
	result = contentType{}
	return
}

func (p *Parser) contentType() (result contentType, err error) {
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

func (p *Parser) mimeType() (str string, err error) {

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

func (p *Parser) mimeSubType() (str string, err error) {
	return p.mimeType()
}

// comment     =  "(" *(ctext / quoted-pair / comment) ")"
//
// ctext       =  <any CHAR excluding "(",     ; => may be folded
//                     ")", "\" & CR, & including
//                     linear-white-space>
//
// quoted-pair =  "\" CHAR                     ; may quote any char
func (p *Parser) comment() (err error) {
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

func (p *Parser) token() (str string, err error) {
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
func (p *Parser) quotedString() (str string, err error) {
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
func (p *Parser) parameter() (attribute, value string, err error) {
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

// isBranch determines if we should branch this part, when building
// the mime tree
func (p *Parser) isBranch(part *Part, parent *Part) bool {
	ct := part.ContentType
	if ct == nil {
		return false
	}
	if part.ContentBoundary == "" {
		return false
	}

	// tolerate some incorrect messages that re-use the identical content-boundary
	if parent != nil && ct.superType != "message" {
		if parent.ContentBoundary == part.ContentBoundary {
			return false
		}
	}

	// branch on these superTypes
	if ct.superType == "multipart" ||
		ct.superType == "message" {
		return true
	}
	return false
}

// multi finds the boundary and call back to mime() itself
func (p *Parser) multi(part *Part, depth string) (err error) {
	if part.ContentType != nil {
		// scan until the start of the boundary
		if part.ContentType.superType == "multipart" {
			if end, bErr := p.boundary(part.ContentBoundary); bErr != nil {
				return bErr
			} else if end {
				part.EndingPosBody = p.lastBoundaryPos
				return
			}
		}
		// call back to mime() to start working on a new branch
		err = p.mime(part, depth)
		if err != nil {
			return err
		}
	}
	return
}

// mime scans the mime content and builds the mime-part tree in
// p.Parts on-the-fly, as more bytes get fed in.
func (p *Parser) mime(parent *Part, depth string) (err error) {

	count := 1
	for {
		part := newPart()
		part.StartingPos = p.msgPos

		// parse the headers
		if p.ch >= 33 && p.ch <= 126 {
			err = p.header(part)
			if err != nil {
				return err
			}
		} else {
			return errors.New("parse error")
		}
		if p.ch == '\n' && p.peek() == '\n' {
			p.next()
			p.next()
		}

		// inherit the content boundary from parent if not present
		if part.ContentBoundary == "" && parent != nil {
			part.ContentBoundary = parent.ContentBoundary
		}

		// record the part
		part.StartingPosBody = p.msgPos
		partID := strconv.Itoa(count)
		if depth != "" {
			partID = depth + "." + strconv.Itoa(count)
		}
		p.addPart(part, partID)

		// build the mime tree recursively
		if p.isBranch(part, parent) {
			err = p.multi(part, partID)
			part.EndingPosBody = p.lastBoundaryPos
			if err != nil {
				break
			}
		}

		// if we didn't branch & we're still at the root (not a mime email)
		if parent == nil {
			for {
				// keep scanning until the end
				p.next()
				if p.ch == 0 {
					break
				}
			}
			part.EndingPosBody = p.msgPos
			err = NotMime
			return
		}

		// after we return from the lower branches (if there were any)
		// we walk each of the siblings of the parent
		if end, bErr := p.boundary(parent.ContentBoundary); bErr != nil {
			part.EndingPosBody = p.lastBoundaryPos
			return bErr
		} else if end {
			// the last sibling
			part.EndingPosBody = p.lastBoundaryPos
			return
		}
		part.EndingPosBody = p.lastBoundaryPos
		count++
	}
	return
}

func (p *Parser) reset() {
	p.lastBoundaryPos = 0
	p.pos = startPos
	p.msgPos = 0
	p.msgLine = 0
	p.count = 0
}

// Close tells the MIME Parser there's no more data & waits for it to return a result
// it will return an io.EOF error if no error with parsing MIME was detected
func (p *Parser) Close() error {
	p.Lock()
	defer func() {
		p.reset()
		p.Unlock()
	}()
	if p.count == 0 {
		// already closed
		return nil
	}
	p.gotNewSlice <- false
	r := <-p.result
	return r.err
}

// Parse takes a byte stream, and feeds it to the MIME Parser, then
// waits for the Parser to consume all input before returning.
// The Parser will build a parse tree in p.Parts
// The reader doesn't decode any input. All it does
// is collect information about where the different MIME parts
// start and end, and other meta-data. This data can be used
// by others later to determine how to store/display
// the messages
// returns error if there's a parse error
func (p *Parser) Parse(buf []byte) error {
	defer func() {
		p.count++
		p.Unlock()
	}()
	p.Lock()

	p.set(buf)

	if p.count == 0 {
		//open
		go func() {
			p.next()
			err := p.mime(nil, "")
			fmt.Println("mine() ret", err)
			p.result <- parserMsg{err}
		}()
	} else {
		p.gotNewSlice <- true
	}

	select {
	case <-p.consumed: // wait for prev buf to be consumed
		return nil
	case r := <-p.result:
		// mime() has returned with a result
		p.reset()
		return r.err
	}
}

func NewMimeParser() *Parser {
	p := new(Parser)
	p.consumed = make(chan bool)
	p.gotNewSlice = make(chan bool)
	p.result = make(chan parserMsg, 1)
	return p
}
