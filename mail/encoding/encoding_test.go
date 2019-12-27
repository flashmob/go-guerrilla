package encoding

import (
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
	"testing"
)

// This will use the golang.org/x/net/html/charset encoder
func TestEncodingMimeHeaderDecode(t *testing.T) {
	str := mail.MimeHeaderDecode("=?ISO-2022-JP?B?GyRCIVo9dztSOWJAOCVBJWMbKEI=?=")
	if i := strings.Index(str, "【女子高生チャ"); i != 0 {
		t.Error("expecting 【女子高生チャ, got:", str)
	}
	str = mail.MimeHeaderDecode("=?ISO-8859-1?Q?Andr=E9?= Pirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "André Pirard") != 0 {
		t.Error("expecting André Pirard, got:", str)

	}

	str = mail.MimeHeaderDecode("=?ISO-8859-1?Q?Andr=E9?=\tPirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "André\tPirard") != 0 {
		t.Error("expecting André Pirard, got:", str)

	}

}

// TestEncodingMimeHeaderDecodeEnding tests when the encoded word is at the end
func TestEncodingMimeHeaderDecodeEnding(t *testing.T) {

	// plaintext at the beginning
	str := mail.MimeHeaderDecode("What about this one? =?ISO-8859-1?Q?Andr=E9?=")
	if str != "What about this one? André" {
		t.Error("expecting: What about this one? André, but got:", str)

	}

	// not plaintext at beginning
	str = mail.MimeHeaderDecode("=?ISO-8859-1?Q?Andr=E9?= What about this one? =?ISO-8859-1?Q?Andr=E9?=")
	if str != "André What about this one? André" {
		t.Error("expecting: André What about this one? André, but got:", str)

	}
	// plaintext at beginning corruped
	str = mail.MimeHeaderDecode("=?ISO-8859-1?B?Andr=E9?= What about this one? =?ISO-8859-1?Q?Andr=E9?=")
	if strings.Index(str, "=?ISO-8859-1?B?Andr=E9?= What about this one? André") != 0 {
		t.Error("expecting:=?ISO-8859-1?B?Andr=E9?= What about this one? André, but got:", str)

	}
}

// TestEncodingMimeHeaderDecodeBad tests the case of a malformed encoding
func TestEncodingMimeHeaderDecodeBad(t *testing.T) {
	// bad base64 encoding, it should return the string unencoded
	str := mail.MimeHeaderDecode("=?ISO-8859-1?B?Andr=E9?=\tPirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "=?ISO-8859-1?B?Andr=E9?=\tPirard <PIRARD@vm1.ulg.ac.be>") != 0 {
		t.Error("expecting =?ISO-8859-1?B?Andr=E9?=\tPirard <PIRARD@vm1.ulg.ac.be>, got:", str)

	}

}

func TestEncodingMimeHeaderDecodeNoSpace(t *testing.T) {
	// there is no space
	str := mail.MimeHeaderDecode("A =?ISO-8859-1?Q?Andr=E9?=WORLD IN YOUR POCKET")
	if str != "A AndréWORLD IN YOUR POCKET" {
		// in this case, if it's QP and ?= is found at the end then we can assume no space?
		t.Error("Did not get [A AndréWORLD IN YOUR POCKET]")
	}
}

func TestEncodingMimeHeaderDecodeMulti(t *testing.T) {

	str := mail.MimeHeaderDecode("=?iso-2022-jp?B?GyRCIVpLXEZ8Om89fCFbPEIkT0lUOk5NUSROJU0lPyROSn0bKEI=?= =?iso-2022-jp?B?GyRCJCxCPyQkJEckORsoQg==?=")
	if strings.Index(str, "【本日削除】実は不採用のネタの方が多いです") != 0 {
		t.Error("expecting 【本日削除】実は不採用のネタの方が多いです, got:", str)
	}

	str = mail.MimeHeaderDecode("=?iso-2022-jp?B?GyRCIVpLXEZ8Om89fCFbPEIkT0lUOk5NUSROJU0lPyROSn0bKEI=?= \t =?iso-2022-jp?B?GyRCJCxCPyQkJEckORsoQg==?=")
	if strings.Index(str, "【本日削除】実は不採用のネタの方が多いです") != 0 {
		t.Error("expecting 【本日削除】実は不採用のネタの方が多いです, got:", str)
	}
}
