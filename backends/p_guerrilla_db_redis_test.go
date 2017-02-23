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
	fmt.Fprint(&out, cd)

	// decompress
	var result bytes.Buffer
	zReader, _ := zlib.NewReader(bytes.NewReader(out.Bytes()))
	io.Copy(&result, zReader)
	expect := sbj + str
	if delta := strings.Compare(expect, result.String()); delta != 0 {
		t.Error(delta, "compression did match, expected", expect, "but got", result.String())
	}

}
