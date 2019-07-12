package mime

/*

Mime is a simple MIME scanner for email-message byte streams.
It builds a data-structure that represents a tree of all the mime parts,
recording their headers, starting and ending positions, while processinging
the message efficiently, slice by slice. It avoids the use of regular expressions,
doesn't back-track or multi-scan.

This package used the PECL Mailparse library as a refrence/benchmark for testing

*/
import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

const (
	maxBoundaryLen       = 70 + 10
	doubleDash           = "--"
	startPos             = -1
	headerErrorThreshold = 4
)

type boundaryEnd struct {
	cb string
}

func (e boundaryEnd) Error() string {
	return e.cb
}

var NotMime = errors.New("not Mime")

type captureBuffer struct {
	bytes.Buffer
	upper bool
}

type Parser struct {

	// related to the state of the parser

	buf                   []byte
	pos                   int
	peekOffset            int
	ch                    byte
	gotNewSlice, consumed chan bool
	accept                captureBuffer
	boundaryMatched       int
	count                 uint
	result                chan parserMsg
	mux                   sync.Mutex

	// Parts is the mime parts tree. The parser builds the parts as it consumes the input
	// In order to represent the tree in an array, we use Parts.Node to store the name of
	// each node. The name of the node is the *path* of the node. The root node is always
	// "1". The child would be "1.1", the next sibling would be "1.2", while the child of
	// "1.2" would be "1.2.1"
	Parts           []*Part
	msgPos          uint
	lastBoundaryPos uint
}

type Part struct {
	Headers textproto.MIMEHeader

	Node string // stores the name for the node that is a part of the resulting mime tree

	StartingPos     uint // including header (after boundary, 0 at the top)
	StartingPosBody uint // after header \n\n
	EndingPos       uint // redundant (same as endingPos)
	EndingPosBody   uint // the char before the boundary marker

	Charset          string
	TransferEncoding string
	ContentBoundary  string
	ContentType      *contentType
	ContentBase      string

	DispositionFi1eName string
	ContentDisposition  string
	ContentName         string
}

type parameter struct {
	name  string
	value string
}

type contentType struct {
	superType  string
	subType    string
	parameters []parameter
	b          bytes.Buffer
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

func (c *contentType) params() (ret string) {
	defer func() {
		c.b.Reset()
	}()
	for k := range c.parameters {
		if c.parameters[k].value == "" {
			c.b.WriteString("; " + c.parameters[k].name)
			continue
		}
		c.b.WriteString("; " + c.parameters[k].name + "=\"" + c.parameters[k].value + "\"")
	}
	return c.b.String()
}

func (c *contentType) String() (ret string) {
	ret = fmt.Sprintf("%s/%s%s", c.superType, c.subType,
		c.params())
	return
}

func newPart() *Part {
	mh := new(Part)
	mh.Headers = make(textproto.MIMEHeader, 1)
	return mh
}

func (p *Parser) addPart(mh *Part, id string) {
	mh.Node = id
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
	for {
		// wait for more bytes if reached the end
		if p.pos+1 >= len(p.buf) {
			if !p.more() {
				p.ch = 0
				return 0
			}
		}
		if p.pos > -1 || p.msgPos != 0 {
			// dont incr on first call to next()
			p.msgPos++
		}
		p.pos++
		if p.buf[p.pos] == '\r' {
			// ignore \r
			continue
		}
		p.ch = p.buf[p.pos]

		return p.ch
	}
}

// peek does not advance the pointer, but will block if there's no more
// input in the buffer
func (p *Parser) peek() byte {
	p.peekOffset = 1
	for {
		// reached the end? Wait for more bytes to consume
		if p.pos+p.peekOffset >= len(p.buf) {
			if !p.more() {
				return 0
			}
		}
		// peek the next byte
		ret := p.buf[p.pos+p.peekOffset]
		if ret == '\r' {
			// ignore \r
			p.peekOffset++
			continue
		}
		return ret
	}
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

// Set the buffer and reset p.pos to startPos, which is typically -1
// The reason why -1 is because peek() implementation becomes more
// simple, as it only needs to add 1 to p.pos for all cases.
// We don't read the buffer when we set, only when next() is called.
// This allows us to peek in to the next buffer while still being on
// the last element from the previous buffer
func (p *Parser) set(input []byte) {
	if p.pos != startPos {
		// rewind
		p.pos = startPos
	}
	p.buf = input
}

// skip advances the pointer n bytes. It will block if not enough bytes left in
// the buffer, i.e. if bBytes > len(p.buf) - p.pos
func (p *Parser) skip(nBytes int) {
	for {
		if p.pos+nBytes < len(p.buf) {
			p.pos += nBytes - 1
			p.msgPos = p.msgPos + uint(nBytes) - 1
			p.next()
			return
		}
		remainder := len(p.buf) - p.pos
		nBytes -= remainder
		p.pos += remainder - 1
		p.msgPos += uint(remainder - 1)
		p.next()
		if nBytes < 1 {
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

			p.skip(i)
			p.lastBoundaryPos = p.msgPos // -1 - uint(len(boundary))
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

			p.skip(len(p.buf) - p.pos) // discard the remaining data

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

					p.lastBoundaryPos = p.msgPos - uint(len(boundary)) // -1
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
			if c == 0 {
				err = io.EOF
			}
			return
		}
	}
}

// acceptHeaderName build the header name in the buffer while ensuring that
// that the case is normalized. Ie. Content-type is written as Content-Type
func (p *Parser) acceptHeaderName() {
	if p.accept.upper && p.ch >= 'a' && p.ch <= 'z' {
		p.ch -= 32
	}
	if !p.accept.upper && p.ch >= 'A' && p.ch <= 'Z' {
		p.ch += 32
	}
	p.accept.upper = p.ch == '-'
	_ = p.accept.WriteByte(p.ch)
}

func (p *Parser) header(mh *Part) (err error) {
	var (
		state      int
		name       string
		errorCount int
	)

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
		case 0: // header name
			if (p.ch >= 33 && p.ch <= 126) && p.ch != ':' {
				// capture
				p.acceptHeaderName()
			} else if p.ch == ':' {
				state = 1
			} else if p.ch == ' ' && p.peek() == ':' { // tolerate a SP before the :
				p.next()
				state = 1
			} else {
				if errorCount < headerErrorThreshold {
					state = 2 // tolerate this error
					continue
				}
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
				p.accept.upper = true
				name = p.accept.String()
				p.accept.Reset()
				if c := p.peek(); c == ' ' {
					// skip the space
					p.next()
				}
				p.next()
				continue
			}

		case 1: // header value

			if name == "Content-Type" {
				var err error
				contentType, err := p.contentType()
				if err != nil {
					return err
				}
				mh.ContentType = &contentType
				for i := range contentType.parameters {
					switch {
					case contentType.parameters[i].name == "boundary":
						mh.ContentBoundary = contentType.parameters[i].value
					case contentType.parameters[i].name == "charset":
						mh.Charset = contentType.parameters[i].value
					case contentType.parameters[i].name == "name":
						mh.ContentName = contentType.parameters[i].value
					}
				}
				mh.Headers.Add("Content-Type", contentType.String())
				state = 0
			} else {
				if (p.ch >= 33 && p.ch <= 126) || p.isWSP(p.ch) {
					_ = p.accept.WriteByte(p.ch)
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
					err = errors.New("header parse error, pos:" + strconv.Itoa(p.pos))
					return
				}
			}
		case 2: // header error, discard line
			errorCount++
			// error recovery for header lines with parse errors -
			// ignore the line, discard anything that was scanned, scan until the end-of-line
			// then start a new line again (back to state 0)
			p.accept.Reset()
			for {
				if p.ch != '\n' {
					p.next()
				}
				if p.ch == 0 {
					return io.EOF
				} else if p.ch == '\n' {
					state = 0
					break
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

	for {
		if p.ch == ';' {
			p.next()
			continue
		}
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
				// add the new parameter
				result.parameters = append(result.parameters, parameter{key, val})
			}
		} else {
			break
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
			if p.ch >= 'A' && p.ch <= 'Z' {
				p.ch += 32 // lowercase
			}
			_ = p.accept.WriteByte(p.ch)
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

func (p *Parser) token(lower bool) (str string, err error) {
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
			if lower && p.ch >= 'A' && p.ch <= 'Z' {
				p.ch += 32 // lowercase it
			}
			_ = p.accept.WriteByte(p.ch)
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
				_ = p.accept.WriteByte(p.ch)
			} else {
				err = errors.New("unexpected token")
				return
			}
		case 1:
			// escaped (<any US-ASCII character (octets 0 - 127)>)
			if p.ch != 0 && p.ch <= 127 {
				_ = p.accept.WriteByte(p.ch)
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

	if attribute, err = p.token(true); err != nil {
		return "", "", err
	}
	if p.ch != '=' {
		if len(attribute) > 0 {
			return
		}
		return "", "", errors.New("expecting =")
	}
	p.next()
	if p.ch == '"' {
		if value, err = p.quotedString(); err != nil {
			return
		}
		return
	} else {
		if value, err = p.token(false); err != nil {
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
	if ct.superType == "message" && ct.subType == "delivery-status" {
		return false
	}
	if ct.superType == "message" && ct.subType == "disposition-notification" {
		return false
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
		err = p.mime2(part, depth)
		if err != nil {
			return err
		}
	}
	return
}

func (p *Parser) mime(depth string, count int, part *Part) (err error) {

	if count == 0 {
		count = 1
	}
	count = 1
	first := part == nil
	if first {
		part = newPart()
		p.addPart(part, "1")
	}

	// read the header
	if p.ch >= 33 && p.ch <= 126 {
		err = p.header(part)
		if err != nil {
			return err
		}
	} else if first {
		return errors.New("parse error, no header")
	}
	if p.ch == '\n' && p.peek() == '\n' {
		p.next()
		p.next()
	}
	ct := part.ContentType
	if ct != nil && ct.superType == "message" && ct.subType == "rfc822" {

		var subPart *Part
		subPart = newPart()
		subPartId := part.Node + "." + strconv.Itoa(count)
		subPart.StartingPos = p.msgPos
		count++
		p.addPart(subPart, subPartId)
		err = p.mime(subPartId, count, subPart)
		return
	}
	if ct != nil && ct.superType == "multipart" && part.ContentBoundary != "" {
		var subPart *Part
		subPart = newPart()
		for {
			subPartId := part.Node + "." + strconv.Itoa(count)
			if end, bErr := p.boundary(part.ContentBoundary); bErr != nil {
				err = bErr
				if subPart.StartingPos == 0 {
					subPart.StartingPos = p.msgPos
				} else {
					//fmt.Println("["+string(p.buf[subPart.StartingPos:p.msgPos])+"]")
					subPart, count = p.split(subPart, count)
				}
				return
			} else if end {
				return
			} else {
				if subPart.StartingPos == 0 {
					subPart.StartingPos = p.msgPos
					count++
					p.addPart(subPart, subPartId)
					err = p.mime(subPartId, count, subPart)
					if err != nil {
						return
					}
					subPartId = part.Node + "." + strconv.Itoa(count)
				} else {
					//fmt.Println("["+string(p.buf[subPart.StartingPos:p.msgPos])+"]")
					subPart, count = p.split(subPart, count)
					//subPart.Node = subPartId
					p.addPart(subPart, subPartId)
					err = p.mime(subPartId, count, subPart)
					if err != nil {
						return
					}
				}
			}
		}
	}
	part.EndingPosBody = p.lastBoundaryPos
	return

}

func (p *Parser) split(subPart *Part, count int) (*Part, int) {
	subPart.EndingPos = p.msgPos
	subPart = nil
	count++
	subPart = newPart()
	subPart.StartingPos = p.msgPos
	return subPart, count
}

func (p *Parser) mime_new(depth string, count int, cb string) (err error) {

	defer func() {
		fmt.Println("i quit")
	}()
	if count == 0 {
		count = 1
	}
	part := newPart()

	partID := strconv.Itoa(count)
	if depth != "" {
		partID = depth + "." + strconv.Itoa(count)
	}
	p.addPart(part, partID)
	// record the start of the part
	part.StartingPos = p.msgPos

	// read the header
	if p.ch >= 33 && p.ch <= 126 {
		err = p.header(part)
		if err != nil {
			return err
		}
	} else if depth == "" {
		return errors.New("parse error, no header")
	}
	if p.ch == '\n' && p.peek() == '\n' {
		p.next()
		p.next()
	}
	part.StartingPosBody = p.msgPos
	skip := false
	if part.ContentBoundary != "" {
		if cb == part.ContentBoundary {
			// tolerate some messages that have identical multipart content-boundary
			skip = true
		}
		cb = part.ContentBoundary
	}
	ct := part.ContentType
	if part.ContentType != nil && ct.superType == "message" &&
		ct.subType == "rfc822" {

		err = p.mime_new(partID, 1, cb)
		part.EndingPosBody = p.msgPos
		if err != nil {
			return
		}
	}

	for {
		if cb != "" {
			if end, bErr := p.boundary(cb); bErr != nil || end == true {
				part.EndingPosBody = p.lastBoundaryPos
				if end {
					bErr = boundaryEnd{cb}

					return bErr
				}
				return bErr
			}
			part.EndingPosBody = p.msgPos
		} else {
			for p.ch != 0 {
				// keep scanning until the end
				p.next()
			}
			part.EndingPosBody = p.msgPos
			err = NotMime
			return
		}

		if !skip && ct != nil &&
			(ct.superType == "multipart" || (ct.superType == "message" && ct.subType == "rfc822")) {
			// start a new branch (count is 1)
			err = p.mime_new(partID, count, cb)

			part.EndingPosBody = p.msgPos // good?
			if err != nil {
				if v, ok := err.(boundaryEnd); ok && v.Error() != cb {
					// we are back to the upper level, stop propagating the content-boundary 'end' error
					count++
					continue
				}
				if depth == "" {
					part.EndingPosBody = p.lastBoundaryPos
				}

				return
			}

		} else {
			// new sibling for this node
			count++
			err = p.mime_new(depth, count, cb)

			if err == nil {
				return
			}
			if v, ok := err.(boundaryEnd); ok && v.Error() != cb {
				// we are back to the upper level, stop propagating the content-boundary 'end' error
				continue
			}
			return
		}
	}
}

// mime scans the mime content and builds the mime-part tree in
// p.Parts on-the-fly, as more bytes get fed in.
func (p *Parser) mime2(parent *Part, depth string) (err error) {

	count := 1
	for {
		part := newPart()
		partID := strconv.Itoa(count)
		if depth != "" {
			partID = depth + "." + strconv.Itoa(count)
		}
		p.addPart(part, partID)
		// record the start of the part
		part.StartingPos = p.msgPos
		// parse the headers
		if p.ch >= 33 && p.ch <= 126 {
			err = p.header(part)
			if err != nil {
				return err
			}
		} else if len(p.Parts) == 0 {
			// return an error if the first part is not a valid header
			// (subsequent parts could have no headers)
			return errors.New("parse error, no header")
		}
		if p.ch == '\n' && p.peek() == '\n' {
			p.next()
			p.next()
		}

		// inherit the content boundary from parent if not present
		if part.ContentBoundary == "" && parent != nil {
			part.ContentBoundary = parent.ContentBoundary
		}

		// record the start of the message body
		part.StartingPosBody = p.msgPos

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
			for p.ch != 0 {
				// keep scanning until the end
				p.next()

			}
			if len(p.Parts) == 1 {
				part.EndingPosBody = p.msgPos
				err = NotMime
			} else {
				err = io.EOF
			}
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
	p.count = 0
	p.ch = 0
}

func (p *Parser) Open() {
	p.Parts = make([]*Part, 0)
}

// Close tells the MIME Parser there's no more data & waits for it to return a result
// it will return an io.EOF error if no error with parsing MIME was detected
func (p *Parser) Close() error {
	p.mux.Lock()
	defer func() {
		p.reset()
		p.mux.Unlock()
	}()
	if p.count == 0 {
		return nil
	}
	for {
		select {
		// we need to repeat sending a false signal because peek() / next() could be
		// called a few times before a result is returned
		case p.gotNewSlice <- false:
			select {

			case <-p.consumed: // more() was called, there's nothing to consume

			case r := <-p.result:
				return r.err
			}
		case r := <-p.result:

			return r.err
		}

	}

}

// Parse takes a byte stream, and feeds it to the MIME Parser, then
// waits for the Parser to consume all input before returning.
// The parser will build a parse tree in p.Parts
// The parser doesn't decode any input. All it does
// is collect information about where the different MIME parts
// start and end, and other meta-data. This data can be used
// later down the stack to determine how to store/display
// the messages
// returns error if there's a parse error, except io.EOF when no
// error occurred.
func (p *Parser) Parse(buf []byte) error {
	defer func() {

		p.mux.Unlock()
	}()
	p.mux.Lock()

	// Feed the new slice. Assumes that the parser is blocked now, waiting
	// for new data, or not started yet.
	p.set(buf)

	if p.count == 0 {
		// initial step - start the mime parser
		go func() {
			p.next()
			err := p.mime("", 1, nil)
			//err := p.mime2(nil, "")
			if _, ok := err.(boundaryEnd); ok {
				err = nil
			}
			fmt.Println("mine() ret", err)

			p.result <- parserMsg{err}
		}()
	} else {
		// tell the parser to resume consuming
		p.gotNewSlice <- true
	}
	p.count++

	select {
	case <-p.consumed: // wait for prev buf to be consumed
		return nil
	case r := <-p.result:
		// mime() has returned with a result (it finished consuming)
		p.reset()
		return r.err
	}
}

func (p *Parser) ParseError(err error) bool {
	if err != nil && err != io.EOF && err != NotMime {
		return true
	}
	return false
}

func NewMimeParser() *Parser {
	p := new(Parser)
	p.consumed = make(chan bool)
	p.gotNewSlice = make(chan bool)
	p.result = make(chan parserMsg, 1)
	return p
}
