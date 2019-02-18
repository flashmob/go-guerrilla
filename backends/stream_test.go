package backends

import (
	"bytes"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
	"testing"
)

func TestStream(t *testing.T) {

	var e = mail.Envelope{
		RcptTo:   []mail.Address{{User: "test", Host: "example.com"}},
		Helo:     "a.cool.host.com",
		RemoteIP: "6.6.4.4",
	}
	hc := HeaderConfig{"sharklasers.com"}

	var buf bytes.Buffer
	dc := newStreamDecompresser(&buf)
	comp := newStreamCompressor(dc)

	s := newStreamHeader(comp)
	s.addHeader(&e, hc)

	n, err := io.Copy(s, bytes.NewBufferString("testing123"))
	if err != nil {
		t.Error(err, n)
	}

	if wc, ok := comp.(io.WriteCloser); ok {
		err = wc.Close()
		fmt.Println("err1", err)
	}

	if wcec, ok := dc.(io.WriteCloser); ok {
		err = wcec.Close()
		fmt.Println("err2", err)
	}

	fmt.Println((buf.String()))

	//time.Sleep(time.Second * 10)
}
