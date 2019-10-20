package mime

/*

Mime is a simple MIME scanner for email-message byte streams.
It builds a data-structure that represents a tree of all the mime parts,
recording their headers, starting and ending positions, while processioning
the message efficiently, slice by slice. It avoids the use of regular expressions,
doesn't back-track or multi-scan.

*/
import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

const (
	// maxBoundaryLen limits the length of the content-boundary.
	// Technically the limit is 79, but here we are more liberal
	maxBoundaryLen = 70 + 10

	// doubleDash is the prefix for a content-boundary string. It is also added
	// as a postfix to a content-boundary string to signal the end of content parts.
	doubleDash = "--"

	// startPos assigns the pos property when the buffer is set.
	// The reason why -1 is because peek() implementation becomes simpler
	startPos = -1

	// headerErrorThreshold how many errors in the header
	headerErrorThreshold = 4

	multipart         = "multipart"
	contentTypeHeader = "Content-Type"
	dot               = "."
	first             = "1"

	// MaxNodes limits the number of items in the Parts array. Effectively limiting
	// the number of nested calls the parser may make.
	MaxNodes = 512
)

var NotMime = errors.New("not Mime")
var MaxNodesErr = errors.New("too many mime part nodes")

type captureBuffer struct {
	bytes.Buffer
	upper bool // flag used by acceptHeaderName(), if true, the next accepted chr will be uppercase'd
}

type Parser struct {

	// related to the state of the parser

	buf                   []byte         // input buffer
	pos                   int            // position in the input buffer
	peekOffset            int            // peek() ignores \r so we must keep count of how many \r were ignored
	ch                    byte           // value of byte at current pos in buf[]. At EOF, ch == 0
	gotNewSlice, consumed chan bool      // flags that control the synchronisation of reads
	accept                captureBuffer  // input is captured to this buffer to build strings
	boundaryMatched       int            // an offset. Used in cases where the boundary string is split over multiple buffers
	count                 uint           // counts how many times Parse() was called
	result                chan parserMsg // used to pass the result back to the main goroutine
	mux                   sync.Mutex     // ensure calls to Parse() and Close() are synchronized

	// Parts is the mime parts tree. The parser builds the parts as it consumes the input
	// In order to represent the tree in an array, we use Parts.Node to store the name of
	// each node. The name of the node is the *path* of the node. The root node is always
	// "1". The child would be "1.1", the next sibling would be "1.2", while the child of
	// "1.2" would be "1.2.1"
	Parts Parts

	msgPos uint // global position in the message

	lastBoundaryPos uint // the last msgPos where a boundary was detected

	maxNodes int // the desired number of maximum nodes the parser is limited to

	w io.Writer // underlying io.Writer
}

type Parts []*Part

type Part struct {

	// Headers contain the header names and values in a map data-structure
	Headers textproto.MIMEHeader

	// Node stores the name for the node that is a part of the resulting mime tree
	Node string
	// StartingPos is the starting position, including header (after boundary, 0 at the top)
	StartingPos uint
	// StartingPosBody is the starting position of the body, after header \n\n
	StartingPosBody uint
	// EndingPos is the ending position for the part
	EndingPos uint
	// EndingPosBody is thr ending position for the body. Typically identical to EndingPos
	EndingPosBody uint

	StartingPosDelta     uint
	StartingPosBodyDelta uint
	EndingPosDelta       uint
	EndingPosBodyDelta   uint

	// Charset holds the character-set the part is encoded in, eg. us-ascii
	Charset string
	// TransferEncoding holds the transfer encoding that was used to pack the message eg. base64
	TransferEncoding string
	// ContentBoundary holds the unique string that was used to delimit multi-parts, eg. --someboundary123
	ContentBoundary string
	// ContentType holds the mime content type, eg text/html
	ContentType *contentType
	// ContentBase is typically a url
	ContentBase string
	// DispositionFi1eName what file-nme to use for the part, eg. image.jpeg
	DispositionFi1eName string
	// ContentDisposition describes how to display the part, eg. attachment
	ContentDisposition string
	// ContentName as name implies
	ContentName string
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

// String returns the contentType type as a string
func (c *contentType) String() (ret string) {
	ret = fmt.Sprintf("%s/%s%s", c.superType, c.subType,
		c.params())
	return
}

// Charset returns the charset value specified by the content type
func (c *contentType) Charset() (ret string) {
	if c.superType == "" {
		return ""
	}
	for i := range c.parameters {
		if c.parameters[i].name == "charset" {
			return c.parameters[i].value
		}
	}
	return ""
}

func (c *contentType) Supertype() (ret string) {
	return c.superType
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
		if p.ch == 0 {
			return
		} else if nBytes < 1 {
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
			p.lastBoundaryPos = p.msgPos
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

					p.lastBoundaryPos = p.msgPos - uint(len(boundary))
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

// acceptHeaderName builds the header name in the buffer while ensuring that
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
			if name == contentTypeHeader {
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
						if len(mh.ContentBoundary) >= maxBoundaryLen {
							return errors.New("boundary exceeded max length")
						}
					case contentType.parameters[i].name == "charset":
						mh.Charset = strings.ToUpper(contentType.parameters[i].value)
					case contentType.parameters[i].name == "name":
						mh.ContentName = contentType.parameters[i].value
					}
				}
				mh.Headers.Add(contentTypeHeader, contentType.String())
				state = 0
			} else {
				if p.ch != '\n' || p.isWSP(p.ch) {
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
				if key == "charset" {
					val = strings.ToUpper(val)
				}
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

// mime scans the mime content and builds the mime-part tree in
// p.Parts on-the-fly, as more bytes get fed in.
func (p *Parser) mime(part *Part, cb string) (err error) {
	if len(p.Parts) >= p.maxNodes {
		for {
			// skip until the end of the stream (we've stopped parsing due to max nodes)
			p.skip(len(p.buf) + 1)
			if p.ch == 0 {
				break
			}
		}
		if p.maxNodes == 1 {
			// in this case, only one header item, so assume the end of message is
			// the ending position of the header
			p.Parts[0].EndingPos = p.msgPos
			p.Parts[0].EndingPosBody = p.msgPos
		} else {
			err = MaxNodesErr
		}
		return
	}
	count := 1
	root := part == nil
	if root {
		part = newPart()
		p.addPart(part, first)
		defer func() {
			if err != MaxNodesErr {
				part.EndingPosBody = p.lastBoundaryPos
			} else {
				// remove the unfinished node (edge case)
				var parts []*Part
				p.Parts = append(parts, p.Parts[:p.maxNodes]...)
			}
		}()
	}

	// read the header
	if p.ch >= 33 && p.ch <= 126 {
		err = p.header(part)
		if err != nil {
			return err
		}
	} else if root {
		return errors.New("parse error, no header")
	}
	if p.ch == '\n' && p.peek() == '\n' {
		p.next()
		p.next()
	}
	part.StartingPosBody = p.msgPos
	ct := part.ContentType
	if ct != nil && ct.superType == "message" && ct.subType == "rfc822" {
		var subPart *Part
		subPart = newPart()
		subPartId := part.Node + dot + strconv.Itoa(count)
		subPart.StartingPos = p.msgPos
		count++
		p.addPart(subPart, subPartId)
		err = p.mime(subPart, part.ContentBoundary)
		subPart.EndingPosBody = p.msgPos
		part.EndingPosBody = p.msgPos
		return
	}
	if ct != nil && ct.superType == multipart &&
		part.ContentBoundary != "" &&
		part.ContentBoundary != cb { /* content-boundary must be different to previous */
		var subPart *Part
		subPart = newPart()
		subPart.ContentBoundary = part.ContentBoundary
		for {
			subPartId := part.Node + dot + strconv.Itoa(count)
			if end, bErr := p.boundary(part.ContentBoundary); bErr != nil {
				// there was an error with parsing the boundary
				err = bErr
				if subPart.StartingPos == 0 {
					subPart.StartingPos = p.msgPos
				} else {
					subPart.EndingPos = p.msgPos
					subPart.EndingPosBody = p.lastBoundaryPos
					subPart, count = p.split(subPart, count)
				}
				return
			} else if end {
				// reached the terminating boundary (ends with double dash --)
				subPart.EndingPosBody = p.lastBoundaryPos
				break
			} else {
				// process the part boundary
				if subPart.StartingPos == 0 {
					subPart.StartingPos = p.msgPos
					count++
					p.addPart(subPart, subPartId)
					err = p.mime(subPart, part.ContentBoundary)
					if err != nil {
						return
					}
					subPartId = part.Node + dot + strconv.Itoa(count)
				} else {
					subPart.EndingPosBody = p.lastBoundaryPos
					subPart, count = p.split(subPart, count)
					p.addPart(subPart, subPartId)
					err = p.mime(subPart, part.ContentBoundary)
					if err != nil {
						return
					}
				}
			}
		}
	} else if part.ContentBoundary == "" {
		for {
			p.skip(len(p.buf))
			if p.ch == 0 {
				if part.StartingPosBody > 0 {
					part.EndingPosBody = p.msgPos
					part.EndingPos = p.msgPos
				}
				err = io.EOF
				return
			}
		}

	}
	return

}

func (p *Parser) split(subPart *Part, count int) (*Part, int) {
	cb := subPart.ContentBoundary
	subPart = nil
	count++
	subPart = newPart()
	subPart.StartingPos = p.msgPos
	subPart.ContentBoundary = cb
	return subPart, count
}

func (p *Parser) reset() {
	p.lastBoundaryPos = 0
	p.pos = startPos
	p.msgPos = 0
	p.count = 0
	p.ch = 0
}

// Open prepares the parser for accepting input
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

func (p *Parser) Write(buf []byte) (int, error) {
	if err := p.Parse(buf); err != nil {
		return len(buf), err
	}
	if p.w != nil {
		return p.w.Write(buf)
	}
	return len(buf), nil
}

// Parse takes a byte stream, and feeds it to the MIME Parser, then
// waits for the Parser to consume all input before returning.
// The parser will build a parse tree in p.Parts
// The parser doesn't decode any input. All it does
// is collect information about where the different MIME parts
// start and end, and other meta-data. This data can be used
// later down the stack to determine how to store/decode/display
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
			err := p.mime(nil, "")
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

// ParseError returns true if the type of error was a parse error
// Returns false if it was an io.EOF or the email was not mime, or exceeded maximum nodes
func (p *Parser) ParseError(err error) bool {
	if err != nil && err != io.EOF && err != NotMime && err != MaxNodesErr {
		return true
	}
	return false
}

// NewMimeParser returns a mime parser. See MaxNodes for how many nodes it's limited to
func NewMimeParser() *Parser {
	p := new(Parser)
	p.consumed = make(chan bool)
	p.gotNewSlice = make(chan bool)
	p.result = make(chan parserMsg, 1)
	p.maxNodes = MaxNodes
	return p
}

func NewMimeParserWriter(w io.Writer) *Parser {
	p := NewMimeParser()
	p.w = w
	return p
}

// NewMimeParser returns a mime parser with a custom MaxNodes value
func NewMimeParserLimited(maxNodes int) *Parser {
	p := NewMimeParser()
	p.maxNodes = maxNodes
	return p
}
