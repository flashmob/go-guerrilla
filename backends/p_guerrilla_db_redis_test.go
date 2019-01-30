package backends

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestCompressedData(t *testing.T) {
	var b bytes.Buffer
	var out bytes.Buffer
	str := "Hello Hello Hello Hello Hello Hello Hello!"
	sbj := "Subject:hello\r\n"
	b.WriteString(str)
	cd := newCompressedData()
	cd.set([]byte(sbj), &b)

	// compress
	if _, err := fmt.Fprint(&out, cd); err != nil {
		t.Error(err)
	}

	// decompress
	var result bytes.Buffer
	zReader, _ := zlib.NewReader(bytes.NewReader(out.Bytes()))
	if _, err := io.Copy(&result, zReader); err != nil {
		t.Error(err)
	}
	expect := sbj + str
	if delta := strings.Compare(expect, result.String()); delta != 0 {
		t.Error(delta, "compression did match, expected", expect, "but got", result.String())
	}

}
