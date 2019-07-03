package mime

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"
)

var p *Parser

func init() {

}
func TestInject(t *testing.T) {
	p = NewMimeParser()
	var b bytes.Buffer

	// it should read from both slices
	// as if it's a continuous stream
	p.inject([]byte("abcd"), []byte("efgh"), []byte("ijkl"))
	for i := 0; i < 12; i++ {
		b.WriteByte(p.ch)
		if p.pos == 3 && p.msgPos < 4 {
			if c := p.peek(); c != 101 {
				t.Error("next() expecting e, got:", string(c))
			}
		}
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
	p = NewMimeParser()
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
	p = NewMimeParser()
	go func() {
		<-p.consumed
		p.gotNewSlice <- false
	}()
	subject := "text/plain; charset=\"us-ascii\"; moo; boundary=\"foo\""
	p.inject([]byte(subject))
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

	if ct := contentType.String(); contentType.String() != subject {
		t.Error("\n[" + ct + "]\ndoes not equal\n[" + subject + "]")
	}
}

func TestEmailHeader(t *testing.T) {
	p = NewMimeParser()
	in := `Wong ignore me
From: Al Gore <vice-president@whitehouse.gov>
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
	h := newPart()
	err := p.header(h)
	if err != nil {
		t.Error(err)
	}
	if _, err := p.boundary(h.ContentBoundary); err != nil {
		t.Error(err)
	}
}

func TestBoundary(t *testing.T) {
	p = NewMimeParser()
	var err error
	part := newPart()
	part.ContentBoundary = "-wololo-"

	// in the middle of the string
	test := "The quick brown fo---wololo-\nx jumped over the lazy dog"
	p.inject([]byte(test))

	_, err = p.boundary(part.ContentBoundary)
	if err != nil && err != io.EOF {
		t.Error(err)
	}
	body := string(test[:p.lastBoundaryPos])
	if body != "The quick brown fo" {
		t.Error("p.lastBoundaryPos seems incorrect")
	}

	// at the end (with the -- postfix)
	p.inject([]byte("The quick brown fox jumped over the lazy dog---wololo---\n"))
	_, err = p.boundary(part.ContentBoundary)
	if err != nil && err != io.EOF {
		t.Error(err)
	}

	// the boundary with an additional buffer in between
	p.inject([]byte("The quick brown fox jumped over the lazy dog"),
		[]byte("this is the middle"),
		[]byte("and thats the end---wololo---\n"))

	_, err = p.boundary(part.ContentBoundary)
	if err != nil && err != io.EOF {
		t.Error(err)
	}

}

func TestBoundarySplit(t *testing.T) {
	p = NewMimeParser()
	var err error
	part := newPart()

	part.ContentBoundary = "-wololo-"
	// boundary is split over multiple slices
	p.inject(
		[]byte("The quick brown fox jumped ov---wolo"),
		[]byte("lo---\ner the lazy dog"))
	_, err = p.boundary(part.ContentBoundary)
	if err != nil && err != io.EOF {
		t.Error(err)
	}

	body := string([]byte("The quick brown fox jumped ov---wolo")[:p.lastBoundaryPos])
	if body != "The quick brown fox jumped ov" {
		t.Error("p.lastBoundaryPos value seems incorrect")
	}

	// boundary has a space, pointer advanced before, and is split over multiple slices
	part.ContentBoundary = "XXXXboundary text" // 17 chars
	p.inject(
		[]byte("The quick brown fox jumped ov--X"),
		[]byte("XXXboundary text\ner the lazy dog"))
	p.next() // here the pointer is advanced before the boundary is searched
	_, err = p.boundary(part.ContentBoundary)
	if err != nil && err != io.EOF {
		t.Error(err)
		return
	}

}

func TestSkip(t *testing.T) {
	p = NewMimeParser()
	p.inject(
		[]byte("you cant touch this"),
		[]byte("stop, hammer time"))

	p.skip(3)

	if p.pos != 3 {
		t.Error("position should be 3 after skipping 3 bytes, it is:", p.pos)
	}

	p.pos = 0

	// after we used next() to advance
	p.next()
	p.skip(3)
	if p.pos != 4 {
		t.Error("position should be 4 after skipping 3 bytes, it is:", p.pos)
	}

	// advance to the 2nd buffer
	p.pos = 0
	p.msgPos = 0
	p.skip(19)
	if p.pos != 0 && p.buf[0] != 's' {
		t.Error("position should be 0 and p.buf[0] should be 's'")
	}

}

func TestHeaderNormalization(t *testing.T) {
	p = NewMimeParser()
	p.inject([]byte("ConTent-type"))
	p.accept.upper = true
	for {
		p.acceptHeaderName()
		p.next()
		if p.ch == 0 {
			break
		}
	}
	if p.accept.String() != "Content-Type" {
		t.Error("header name not normalized, expecting Content-Type")
	}
}

func TestMimeContentQuotedParams(t *testing.T) {
	p = NewMimeParser()
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
		m := make(map[string]string)
		for _, e := range contentType.parameters {
			m[e.name] = e.value
		}
		if _, ok := m["charset"]; !ok {
			t.Error("charset parameter not present")
		}
		if b, ok := m["boundary"]; !ok {
			t.Error("charset parameter not present")
		} else {
			if b != "D7F------------D7FD5A0B8AB9C65CCDBFA872" {
				t.Error("boundary should be: D7F------------D7FD5A0B8AB9C65CCDBFA872")
			}
		}

	}

}

var email = `From:  Al Gore <vice-president@whitehouse.gov>
To:  White House Transportation Coordinator <transport@whitehouse.gov>
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

Fire up Air Force One!  We're going South!

Thanks,
Al
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: message/rfc822
Content-Transfer-Encoding: 7bit
Content-Disposition: inline

Return-Path: <president@whitehouse.gov>
Received: from mailhost.whitehouse.gov ([192.168.51.200])
 by heartbeat.whitehouse.gov (8.8.8/8.8.8) with ESMTP id SAA22453
 for <vice-president@heartbeat.whitehouse.gov>;
 Mon, 13 Aug 1998 l8:14:23 +1000
Received: from the_big_box.whitehouse.gov ([192.168.51.50])
 by mailhost.whitehouse.gov (8.8.8/8.8.7) with ESMTP id RAA20366
 for vice-president@whitehouse.gov; Mon, 13 Aug 1998 17:42:41 +1000
 Date: Mon, 13 Aug 1998 17:42:41 +1000
Message-Id: <199804130742.RAA20366@mai1host.whitehouse.gov>
From: Bill Clinton <president@whitehouse.gov>
To: A1 (The Enforcer) Gore <vice-president@whitehouse.gov>
Subject:  Map of Argentina with Description
MIME-Version: 1.0
Content-Type: multipart/mixed;
 boundary="DC8------------DC8638F443D87A7F0726DEF7"

This is a multi-part message in MIME format.
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: text/plain; charset=us-ascii
Content-Transfer-Encoding: 7bit

Hi A1,

I finally figured out this MIME thing.  Pretty cool.  I'll send you
some sax music in .au files next week!

Anyway, the attached image is really too small to get a good look at
Argentina.  Try this for a much better map:

http://www.1one1yp1anet.com/dest/sam/graphics/map-arg.htm

Then again, shouldn't the CIA have something like that?

Bill
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: image/gif; name="map_of_Argentina.gif"
Content-Transfer-Encoding: base64
Content-Disposition: in1ine; fi1ename="map_of_Argentina.gif"

R01GOD1hJQA1AKIAAP/////78P/omn19fQAAAAAAAAAAAAAAACwAAAAAJQA1AAAD7Qi63P5w
wEmjBCLrnQnhYCgM1wh+pkgqqeC9XrutmBm7hAK3tP31gFcAiFKVQrGFR6kscnonTe7FAAad
GugmRu3CmiBt57fsVq3Y0VFKnpYdxPC6M7Ze4crnnHum4oN6LFJ1bn5NXTN7OF5fQkN5WYow
BEN2dkGQGWJtSzqGTICJgnQuTJN/WJsojad9qXMuhIWdjXKjY4tenjo6tjVssk2gaWq3uGNX
U6ZGxseyk8SasGw3J9GRzdTQky1iHNvcPNNI4TLeKdfMvy0vMqLrItvuxfDW8ubjueDtJufz
7itICBxISKDBgwgTKjyYAAA7
--DC8------------DC8638F443D87A7F0726DEF7--

--D7F------------D7FD5A0B8AB9C65CCDBFA872--

`

var email2 = `From: abc@def.de
Content-Type: multipart/mixed;
        boundary="----_=_NextPart_001_01CBE273.65A0E7AA"
To: ghi@def.de

This is a multi-part message in MIME format.

------_=_NextPart_001_01CBE273.65A0E7AA
Content-Type: multipart/alternative;
        boundary="----_=_NextPart_002_01CBE273.65A0E7AA"


------_=_NextPart_002_01CBE273.65A0E7AA
Content-Type: text/plain;
        charset="UTF-8"
Content-Transfer-Encoding: base64

[base64-content]
------_=_NextPart_002_01CBE273.65A0E7AA
Content-Type: text/html;
        charset="UTF-8"
Content-Transfer-Encoding: base64

[base64-content]
------_=_NextPart_002_01CBE273.65A0E7AA--
------_=_NextPart_001_01CBE273.65A0E7AA
Content-Type: message/rfc822
Content-Transfer-Encoding: 7bit

X-MimeOLE: Produced By Microsoft Exchange V6.5
Content-class: urn:content-classes:message
MIME-Version: 1.0
Content-Type: multipart/mixed;
        boundary="----_=_NextPart_003_01CBE272.13692C80"
From: bla@bla.de
To: xxx@xxx.de

This is a multi-part message in MIME format.

------_=_NextPart_003_01CBE272.13692C80
Content-Type: multipart/alternative;
        boundary="----_=_NextPart_004_01CBE272.13692C80"


------_=_NextPart_004_01CBE272.13692C80
Content-Type: text/plain;
        charset="iso-8859-1"
Content-Transfer-Encoding: quoted-printable

=20

Viele Gr=FC=DFe

------_=_NextPart_004_01CBE272.13692C80
Content-Type: text/html;
        charset="iso-8859-1"
Content-Transfer-Encoding: quoted-printable

<html>...</html>
------_=_NextPart_004_01CBE272.13692C80--
------_=_NextPart_003_01CBE272.13692C80
Content-Type: application/x-zip-compressed;
        name="abc.zip"
Content-Transfer-Encoding: base64
Content-Disposition: attachment;
        filename="abc.zip"

[base64-content]

------_=_NextPart_003_01CBE272.13692C80--
------_=_NextPart_001_01CBE273.65A0E7AA--`

// note: this mime has an error: the boundary for multipart/alternative is re-used.
// it should use a new unique boundary marker, which then needs to be terminated after
// the text/html part.
var email3 = `MIME-Version: 1.0
X-Mailer: MailBee.NET 8.0.4.428
Subject: test subject
To: kevinm@datamotion.com
Content-Type: multipart/mixed;
       boundary="XXXXboundary text"

--XXXXboundary text
Content-Type: multipart/alternative;
       boundary="XXXXboundary text"

--XXXXboundary text
Content-Type: text/plain;
       charset="utf-8"
Content-Transfer-Encoding: quoted-printable

This is the body text of a sample message.
--XXXXboundary text
Content-Type: text/html;
       charset="utf-8"
Content-Transfer-Encoding: quoted-printable

<pre>This is the body text of a sample message.</pre>

--XXXXboundary text
Content-Type: text/plain;
       name="log_attachment.txt"
Content-Disposition: attachment;
       filename="log_attachment.txt"
Content-Transfer-Encoding: base64

TUlNRS1WZXJzaW9uOiAxLjANClgtTWFpbGVyOiBNYWlsQmVlLk5FVCA4LjAuNC40MjgNClN1Ympl
Y3Q6IHRlc3Qgc3ViamVjdA0KVG86IGtldmlubUBkYXRhbW90aW9uLmNvbQ0KQ29udGVudC1UeXBl
OiBtdWx0aXBhcnQvYWx0ZXJuYXRpdmU7DQoJYm91bmRhcnk9Ii0tLS09X05leHRQYXJ0XzAwMF9B
RTZCXzcyNUUwOUFGLjg4QjdGOTM0Ig0KDQoNCi0tLS0tLT1fTmV4dFBhcnRfMDAwX0FFNkJfNzI1
RTA5QUYuODhCN0Y5MzQNCkNvbnRlbnQtVHlwZTogdGV4dC9wbGFpbjsNCgljaGFyc2V0PSJ1dGYt
OCINCkNvbnRlbnQtVHJhbnNmZXItRW5jb2Rpbmc6IHF1b3RlZC1wcmludGFibGUNCg0KdGVzdCBi
b2R5DQotLS0tLS09X05leHRQYXJ0XzAwMF9BRTZCXzcyNUUwOUFGLjg4QjdGOTM0DQpDb250ZW50
LVR5cGU6IHRleHQvaHRtbDsNCgljaGFyc2V0PSJ1dGYtOCINCkNvbnRlbnQtVHJhbnNmZXItRW5j
b2Rpbmc6IHF1b3RlZC1wcmludGFibGUNCg0KPHByZT50ZXN0IGJvZHk8L3ByZT4NCi0tLS0tLT1f
TmV4dFBhcnRfMDAwX0FFNkJfNzI1RTA5QUYuODhCN0Y5MzQtLQ0K
--XXXXboundary text--
`

/*

email 1
Array
(
    [0] => 1
    [1] => 1.1
    [2] => 1.2
    [3] => 1.2.1
    [4] => 1.2.1.1
    [5] => 1.2.1.2
)
0 =>744 to 3029
1 =>907 to 968
2 =>1101 to 3029
3 =>1889 to 3029
4 =>2052 to 2402
5 =>2594 to 2983

email 2

1  0  121  1763
1.1  207  302  628
1.1.1  343  428  445
1.1.2  485  569  586
1.2  668  730  1763
1.2.1  730  959  1763
1.2.1.1  1045  1140  1501
1.2.1.1.1  1181  1281  1303
1.2.1.1.2  1343  1442  1459
1.2.1.2  1541  1703  1721
*/
func TestNestedEmail(t *testing.T) {
	p = NewMimeParser()
	email = email
	//email = strings.Replace(string(email), "\n", "\r\n", -1)
	p.inject([]byte(email))

	go func() {
		time.Sleep(time.Second * 15)

		// for debugging deadlocks
		//pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		//os.Exit(1)
	}()

	if err := p.mime("", 1, ""); err != nil && err != io.EOF {
		t.Error(err)
	}
	output := email
	for part := range p.Parts {
		//output = replaceAtIndex(output, '#', p.Parts[part].StartingPos)
		//output = replaceAtIndex(output, '&', p.Parts[part].StartingPosBody)
		//output = replaceAtIndex(output, '*', p.Parts[part].EndingPosBody)
		fmt.Println(p.Parts[part].Node + "  " + strconv.Itoa(int(p.Parts[part].StartingPos)) + "  " + strconv.Itoa(int(p.Parts[part].StartingPosBody)) + "  " + strconv.Itoa(int(p.Parts[part].EndingPosBody)))
	}
	fmt.Print(output)
	//fmt.Println(strings.Index(output, "--D7F------------D7FD5A0B8AB9C65CCDBFA872--"))
	i := 1
	_ = i
	//fmt.Println("[" + output[p.Parts[i].StartingPosBody:p.Parts[i].EndingPosBody] + "]")
	//i := 2
	//fmt.Println("**********{" + output[p.parts[i].startingPosBody:p.parts[i].endingPosBody] + "}**********")

	//p.Close()
	//p.inject([]byte(email))
	//if err := p.mime("", 1, ""); err != nil && err != io.EOF {
	//	t.Error(err)
	//}
	//p.Close()
}

func replaceAtIndex(str string, replacement rune, index uint) string {
	return str[:index] + string(replacement) + str[index+1:]
}

var email4 = `Subject: test subject
To: kevinm@datamotion.com

This is not a an MIME email
`

func TestNonMineEmail(t *testing.T) {
	p = NewMimeParser()
	p.inject([]byte(email4))
	if err := p.mime("", 1, ""); err != nil && err != NotMime && err != io.EOF {
		t.Error(err)
	} else {
		for part := range p.Parts {
			fmt.Println(p.Parts[part].Node + "  " + strconv.Itoa(int(p.Parts[part].StartingPos)) + "  " + strconv.Itoa(int(p.Parts[part].StartingPosBody)) + "  " + strconv.Itoa(int(p.Parts[part].EndingPosBody)))
		}
	}
	err := p.Close()
	if err != nil {
		t.Error(err)
	}

	// what if we pass an empty string?
	p.inject([]byte{' '})
	if err := p.mime("", 1, ""); err == nil || err == NotMime || err == io.EOF {
		t.Error("unexpected error", err)
	}

}
