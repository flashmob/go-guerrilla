package backends

import (
	"bytes"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
	"os"
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

func TestHashBytes(t *testing.T) {
	var h HashKey
	h.Pack([]byte{222, 23, 3, 128, 1, 23, 3, 128, 1, 23, 3, 255, 1, 23, 3, 128})
	if h.String() != "3hcDgAEXA4ABFwP/ARcDgA" {
		t.Error("expecting 3hcDgAEXA4ABFwP/ARcDgA got", h.String())
	}
}
func TestChunkSaverWrite(t *testing.T) {

	// place the parse result in an envelope
	e := mail.NewEnvelope("127.0.0.1", 1)
	to, _ := mail.NewAddress("test@test.com")
	e.RcptTo = append(e.RcptTo, to)
	e.MailFrom, _ = mail.NewAddress("test@test.com")

	store := new(chunkSaverMemory)
	chunkBuffer := newChunkedBytesBufferMime()
	//chunkBuffer.setDatabase(store)
	// instantiate the chunk saver
	chunksaver := streamers["chunksaver"]()
	mimeanalyzer := streamers["mimeanalyzer"]()

	// add the default processor as the underlying processor for chunksaver
	// and chain it with mimeanalyzer.
	// Call order: mimeanalyzer -> chunksaver -> default (terminator)
	// This will also set our Open, Close and Initialize functions
	// we also inject a ChunkSaverStorage and a ChunkedBytesBufferMime

	stream := mimeanalyzer.Decorate(chunksaver.Decorate(DefaultStreamProcessor{}, store, chunkBuffer))

	// configure the buffer cap
	bc := BackendConfig{}
	bc["chunksaver_chunk_size"] = 8000
	bc["chunksaver_storage_engine"] = "memory"
	bc["chunksaver_compress_level"] = 0
	_ = Svc.initialize(bc)

	// give it the envelope with the parse results
	_ = chunksaver.Open(e)
	_ = mimeanalyzer.Open(e)

	buf := make([]byte, 128)
	if written, err := io.CopyBuffer(stream, bytes.NewBuffer([]byte(email)), buf); err != nil {
		t.Error(err)
	} else {
		_ = mimeanalyzer.Close()
		_ = chunksaver.Close()
		fmt.Println("written:", written)
		total := 0
		for _, chunk := range store.chunks {
			total += len(chunk.data)
		}
		// 8A9m4qGsTU4wQB1wAgBEVw==
		fmt.Println("compressed", total, "saved:", written-int64(total))
		email, err := store.GetEmail(1)
		if err != nil {
			t.Error("email not found")
			return
		}

		// this should read all parts
		r, err := NewChunkMailReader(store, email, 0)
		if w, err := io.Copy(os.Stdout, r); err != nil {
			t.Error(err)
		} else if w != email.size {
			t.Error("email.size != number of bytes copied from reader")
		}

		// test the seek feature
		r, err = NewChunkMailReader(store, email, 1)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		// we start from 1 because if the start from 0, all the parts will be read
		for i := 1; i < len(email.partsInfo.Parts); i++ {
			fmt.Println("seeking to", i)
			err = r.SeekPart(i)
			if err != nil {
				t.Error(err)
			}
			w, err := io.Copy(os.Stdout, r)
			if err != nil {
				t.Error(err)
			}
			if w != int64(email.partsInfo.Parts[i].Size) {
				t.Error("incorrect size, expecting", email.partsInfo.Parts[i].Size, "but read:", w)
			}
		}

		dr, err := NewChunkPartDecoder(store, email, 5)
		_ = dr
		var decoded bytes.Buffer
		io.Copy(&decoded, dr)

	}
}
