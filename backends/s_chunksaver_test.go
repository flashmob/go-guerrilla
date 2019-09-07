package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"testing"
)

func TestChunkedBytesBuffer(t *testing.T) {
	var in string

	var buf chunkedBytesBuffer
	buf.capTo(64)

	// the data to write is over-aligned
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789abcdef` // len == 130
	i, _ := buf.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is aligned
	var buf2 chunkedBytesBuffer
	buf2.capTo(64)
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789abcd` // len == 128
	i, _ = buf2.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is under-aligned
	var buf3 chunkedBytesBuffer
	buf3.capTo(64)
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789ab` // len == 126
	i, _ = buf3.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is smaller than the buffer
	var buf4 chunkedBytesBuffer
	buf4.capTo(64)
	in = `1234567890` // len == 10
	i, _ = buf4.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// what if the buffer already contains stuff before Write is called
	// and the buffer len is smaller than the len of the slice of bytes we pass it?
	var buf5 chunkedBytesBuffer
	buf5.capTo(5)
	buf5.buf = append(buf5.buf, []byte{'a', 'b', 'c'}...)
	in = `1234567890` // len == 10
	i, _ = buf5.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
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
Content-Disposition: attachment; filename="map_of_Argentina.gif"

R01GOD1hJQA1AKIAAP/////78P/omn19fQAAAAAAAAAAAAAAACwAAAAAJQA1AAAD7Qi63P5w
wEmjBCLrnQnhYCgM1wh+pkgqqeC9XrutmBm7hAK3tP31gFcAiFKVQrGFR6kscnonTe7FAAad
GugmRu3CmiBt57fsVq3Y0VFKnpYdxPC6M7Ze4crnnHum4oN6LFJ1bn5NXTN7OF5fQkN5WYow
BEN2dkGQGWJtSzqGTICJgnQuTJN/WJsojad9qXMuhIWdjXKjY4tenjo6tjVssk2gaWq3uGNX
U6ZGxseyk8SasGw3J9GRzdTQky1iHNvcPNNI4TLeKdfMvy0vMqLrItvuxfDW8ubjueDtJufz
7itICBxISKDBgwgTKjyYAAA7
--DC8------------DC8638F443D87A7F0726DEF7--

--D7F------------D7FD5A0B8AB9C65CCDBFA872--

`

func TestChunkSaverWrite(t *testing.T) {

	// parse an email
	parser := mime.NewMimeParser()

	// place the parse result in an envelope
	e := mail.NewEnvelope("127.0.0.1", 1)
	to, _ := mail.NewAddress("test@test.com")
	e.RcptTo = append(e.RcptTo, to)
	e.Values["MimeParts"] = &parser.Parts

	// instantiate the chunk saver
	chunksaver := streamers["chunksaver"]()

	// add the default processor as the underlying processor for chunksaver
	// this will also set our Open, Close and Initialize functions
	stream := chunksaver.p(DefaultStreamProcessor{})
	// configure the buffer cap
	bc := BackendConfig{}
	bc["chunksaver_chunk_size"] = 64
	bc["chunksaver_storage_engine"] = "memory"
	_ = Svc.initialize(bc)

	// give it the envelope with the parse results
	_ = chunksaver.Open(e)

	// let's test it

	writeIt(parser, t, stream, 128)

	_ = chunksaver.Close()
	//writeIt(parser, t, stream, 128000)
}

func writeIt(parser *mime.Parser, t *testing.T, stream StreamProcessor, size int) {

	if size > len(email) {
		size = len(email)
	}
	total := 0

	// break up the email in to chunks of size. Feed them through the mime parser
	for msgPos := 0; msgPos < len(email); msgPos += size {
		err := parser.Parse([]byte(email)[msgPos : msgPos+size])
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		// todo: close parser on last chunk! (and finalize save)
		cut := msgPos + size
		if cut > len(email) {
			// the last chunk make be shorter than size
			cut -= cut - len(email)
		}
		i, _ := stream.Write([]byte(email)[msgPos:cut])
		total += i
	}
	if total != len(email) {
		t.Error("short write, total is", total, "but len(email) is", len(email))
	}
}
