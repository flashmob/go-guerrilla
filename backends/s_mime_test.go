package backends

import (
	"bytes"
	"testing"
)

var p *parser

func init() {
	p = newMimeParser()
}
func TestInject(t *testing.T) {
	var b bytes.Buffer

	// it should read from both slices
	// as if it's a continuous stream
	p.inject([]byte("abcd"), []byte("efgh"), []byte("ijkl"))
	for i := 0; i < 12; i++ {
		b.WriteByte(p.ch)
		p.next()
		if p.ch == 0 {
			break
		}
	}
	if b.String() != "abcdefghijkl" {
		t.Error("expecting abcdefghijkl, got:", b.String())
	}
}
func TestMimeType(t *testing.T) {

	if isTokenAlphaDash[byte('9')] {
		t.Error("9 should not be in the set")
	}

	if isTokenSpecial['-'] {
		t.Error("- should not be in the set")
	}

	p.inject([]byte("text/plain; charset=us-ascii"))
	str, err := p.mimeType()
	if err != nil {
		t.Error(err)
	}
	if str != "text" {
		t.Error("mime type should be: text")
	}

}

func TestMimeContentType(t *testing.T) {
	go func() {
		<-p.consumed
		p.gotNewSlice <- false
	}()
	p.inject([]byte("text/plain; charset=us-ascii"))
	contentType, err := p.contentType()
	if err != nil {
		t.Error(err)
	}
	if contentType.subType != "plain" {
		t.Error("contentType.subType expecting 'plain', got:", contentType.subType)
	}

	if contentType.superType != "text" {
		t.Error("contentType.subType expecting 'text', got:", contentType.superType)
	}
}

func TestEmailHeader(t *testing.T) {
	in := `From: Al Gore <vice-president@whitehouse.gov>
To: White House Transportation Coordinator <transport@whitehouse.gov>
Subject: [Fwd: Map of Argentina with Description]
MIME-Version: 1.0
DKIM-Signature: v=1; a=rsa-sha256; c=relaxed; s=ncr424; d=reliancegeneral.co.in;
 h=List-Unsubscribe:MIME-Version:From:To:Reply-To:Date:Subject:Content-Type:Content-Transfer-Encoding:Message-ID; i=prospects@prospects.reliancegeneral.co.in;
 bh=F4UQPGEkpmh54C7v3DL8mm2db1QhZU4gRHR1jDqffG8=;
 b=MVltcq6/I9b218a370fuNFLNinR9zQcdBSmzttFkZ7TvV2mOsGrzrwORT8PKYq4KNJNOLBahswXf
   GwaMjDKT/5TXzegdX/L3f/X4bMAEO1einn+nUkVGLK4zVQus+KGqm4oP7uVXjqp70PWXScyWWkbT
   1PGUwRfPd/HTJG5IUqs=
Content-Type: multipart/mixed;
 boundary="D7F------------D7FD5A0B8AB9C65CCDBFA872"

This is a multi-part message in MIME format.
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: text/plain; charset=us-ascii
Content-Transfer-Encoding: 7bit

Fred,

Fire up Air Force One!  We\'re going South!

Thanks,
Al
--D7F------------D7FD5A0B8AB9C65CCDBFA872
This
`
	p.inject([]byte(in))
	h := NewMimeHeader()
	err := p.header(h)
	if err != nil {
		t.Error(err)
	}
	if err := p.boundary(h); err != nil {
		t.Error(err)
	} else {
		//_ = part
		//p.addPart(part)

		//nextPart := NewMimeHeader()
		//err = p.body(part)
		//if err != nil {
		//	t.Error(err)
		//}
	}
}

func msg() (err error) {
	main := NewMimeHeader()
	err = p.header(main)
	if err != nil {
		return err
	}
	p.addPart(main, "1")
	if main.contentBoundary != "" {
		// it's a message with mime parts
		if err = p.boundary(main); err != nil {
			return err
		}
		if err = p.mimeMsg(main, "1"); err != nil {
			return err
		}
	} else {
		// only contains one part (the body)
		if err := p.body(main); err != nil {
			return err
		}
	}
	p.endBody(main)

	return
}

func TestBoundary(t *testing.T) {
	var err error
	part := NewMimeHeader()
	part.contentBoundary = "-wololo-"

	// in the middle of the string
	p.inject([]byte("The quick brown fo-wololo-x jumped over the lazy dog"))

	err = p.boundary(part)
	if err != nil {
		t.Error(err)
	}

	//for c := p.next(); c != 0; c= p.next() {} // drain

	p.inject([]byte("The quick brown fox jumped over the lazy dog-wololo-"))
	err = p.boundary(part)
	if err != nil {
		t.Error(err)
	}

	for c := p.next(); c != 0; c = p.next() {
	} // drain

	// boundary is split over multiple slices
	p.inject(
		[]byte("The quick brown fox jumped ov-wolo"),
		[]byte("lo-er the lazy dog"))
	err = p.boundary(part)
	if err != nil {
		t.Error(err)
	}
	for c := p.next(); c != 0; c = p.next() {
	} // drain
	// the boundary with an additional buffer in between
	p.inject([]byte("The quick brown fox jumped over the lazy dog"),
		[]byte("this is the middle"),
		[]byte("and thats the end-wololo-"))

	err = p.boundary(part)
	if err != nil {
		t.Error(err)
	}

}

func TestMimeContentQuotedParams(t *testing.T) {

	// quoted
	p.inject([]byte("text/plain; charset=\"us-ascii\""))
	contentType, err := p.contentType()
	if err != nil {
		t.Error(err)
	}

	// with whitespace & tab
	p.inject([]byte("text/plain; charset=\"us-ascii\"  \tboundary=\"D7F------------D7FD5A0B8AB9C65CCDBFA872\""))
	contentType, err = p.contentType()
	if err != nil {
		t.Error(err)
	}

	// with comment (ignored)
	p.inject([]byte("text/plain; charset=\"us-ascii\" (a comment) \tboundary=\"D7F------------D7FD5A0B8AB9C65CCDBFA872\""))
	contentType, err = p.contentType()

	if contentType.subType != "plain" {
		t.Error("contentType.subType expecting 'plain', got:", contentType.subType)
	}

	if contentType.superType != "text" {
		t.Error("contentType.subType expecting 'text', got:", contentType.superType)
	}

	if len(contentType.parameters) != 2 {
		t.Error("expecting 2 elements in parameters")
	} else {
		if _, ok := contentType.parameters["charset"]; !ok {
			t.Error("charset parameter not present")
		}
		if b, ok := contentType.parameters["boundary"]; !ok {
			t.Error("charset parameter not present")
		} else {
			if b != "D7F------------D7FD5A0B8AB9C65CCDBFA872" {
				t.Error("boundary should be: D7F------------D7FD5A0B8AB9C65CCDBFA872")
			}
		}
	}

}
