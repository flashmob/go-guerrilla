package mail

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"
)

// Test MimeHeader decoding, not using iconv
func TestMimeHeaderDecode(t *testing.T) {

	/*
		Normally this would fail if not using iconv
		str := MimeHeaderDecode("=?ISO-2022-JP?B?GyRCIVo9dztSOWJAOCVBJWMbKEI=?=")
		if i := strings.Index(str, "【女子高生チャ"); i != 0 {
			t.Error("expecting 【女子高生チャ, got:", str)
		}
	*/

	str := MimeHeaderDecode("=?utf-8?B?55So5oi34oCcRXBpZGVtaW9sb2d5IGluIG51cnNpbmcgYW5kIGg=?=  =?utf-8?B?ZWFsdGggY2FyZSBlQm9vayByZWFkL2F1ZGlvIGlkOm8=?=  =?utf-8?B?cTNqZWVr4oCd5Zyo572R56uZ4oCcU1BZ5Lit5paH5a6Y5pa5572R56uZ4oCd?=  =?utf-8?B?55qE5biQ5Y+36K+m5oOF?=")
	if i := strings.Index(str, "用户“Epidemiology in nursing and health care eBook read/audio id:oq3jeek”在网站“SPY中文官方网站”的帐号详情"); i != 0 {
		t.Error("\nexpecting \n用户“Epidemiology in nursing and h ealth care eBook read/audio id:oq3jeek”在网站“SPY中文官方网站”的帐号详情\n got:\n", str)
	}
	str = MimeHeaderDecode("=?ISO-8859-1?Q?Andr=E9?= Pirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "André Pirard") != 0 {
		t.Error("expecting André Pirard, got:", str)
	}
}

// TestMimeHeaderDecodeNone tests strings without any encoded words
func TestMimeHeaderDecodeNone(t *testing.T) {
	// in the best case, there will be nothing to decode
	str := MimeHeaderDecode("Andre Pirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "Andre Pirard") != 0 {
		t.Error("expecting Andre Pirard, got:", str)
	}

}

func TestAddressPostmaster(t *testing.T) {
	addr := &Address{User: "postmaster"}
	str := addr.String()
	if str != "postmaster" {
		t.Error("it was not postmaster,", str)
	}
}

func TestAddressNull(t *testing.T) {
	addr := &Address{NullPath: true}
	str := addr.String()
	if str != "" {
		t.Error("it was not empty", str)
	}
}

func TestNewAddress(t *testing.T) {

	addr, err := NewAddress("<hoop>")
	if err == nil {
		t.Error("there should be an error:", err)
	}

	addr, err = NewAddress(`Gogh Fir <tesst@test.com>`)
	if err != nil {
		t.Error("there should be no error:", addr.Host, err)
	}
}

func TestQuotedAddress(t *testing.T) {

	str := `<"  yo-- man wazz'''up? surprise \surprise, this is POSSIBLE@fake.com "@example.com>`
	//str = `<"post\master">`
	addr, err := NewAddress(str)
	if err != nil {
		t.Error("there should be no error:", err)
	}

	str = addr.String()
	// in this case, string should remove the unnecessary escape
	if strings.Contains(str, "\\surprise") {
		t.Error("there should be no \\surprise:", err)
	}

}

func TestAddressWithIP(t *testing.T) {
	str := `<"  yo-- man wazz'''up? surprise \surprise, this is POSSIBLE@fake.com "@[64.233.160.71]>`
	addr, err := NewAddress(str)
	if err != nil {
		t.Error("there should be no error:", err)
	} else if addr.IP == nil {
		t.Error("expecting the address host to be an IP")
	}
}

func TestEnvelope(t *testing.T) {
	e := NewEnvelope("127.0.0.1", 22, 0)

	e.QueuedId = QueuedID(2, 33)
	e.Helo = "helo.example.com"
	e.MailFrom = Address{User: "test", Host: "example.com"}
	e.TLS = true
	e.RemoteIP = "222.111.233.121"
	to := Address{User: "test", Host: "example.com"}
	e.PushRcpt(to)
	if to.String() != "test@example.com" {
		t.Error("to does not equal test@example.com, it was:", to.String())
	}
	// we feed the input through the NewMineDotReader, it will parse the headers while reading the input
	// the input has a single line header and ends with a line with a single .
	in := "Subject: =?utf-8?B?55So5oi34oCcRXBpZGVtaW9sb2d5IGluIG51cnNpbmcgYW5kIGg=?=\n\nThis is a test nbnb nbnb hgghgh nnnbnb nbnbnb nbnbn.\n.\n"
	mdr := NewMimeDotReader(bufio.NewReader(bytes.NewBufferString(in)), 1)
	i, err := io.Copy(&e.Data, mdr)
	if err != nil && err != io.EOF {
		t.Error(err, "cannot copy buffer", i, err)
	}
	// pass the parsed headers to the envelope
	if p := mdr.Parts(); p != nil && len(p) > 0 {
		e.Header = p[0].Headers
	}

	addHead := "Delivered-To: " + to.String() + "\n"
	addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteIP + "])\n"
	e.DeliveryHeader = addHead

	r := e.NewReader()

	data, _ := ioutil.ReadAll(r)
	if len(data) != e.Len() {
		t.Error("e.Len() is incorrect, it shown ", e.Len(), " but we wanted ", len(data))
	}

	if err := e.ParseHeaders(); err != nil && err != io.EOF {
		t.Error("cannot parse headers:", err)
		return
	}
	if e.Subject != "用户“Epidemiology in nursing and h" {
		t.Error("Subject expecting: 用户“Epidemiology in nursing and h, got:", e.Subject)
	}

}

func TestEncodedWordAhead(t *testing.T) {
	str := "=?ISO-8859-1?Q?Andr=E9?= Pirard <PIRARD@vm1.ulg.ac.be>"
	if hasEncodedWordAhead(str, 24) != -1 {
		t.Error("expecting no encoded word ahead")
	}

	str = "=?ISO-8859-1?Q?Andr=E9?= ="
	if hasEncodedWordAhead(str, 24) != -1 {
		t.Error("expecting no encoded word ahead")
	}

	str = "=?ISO-8859-1?Q?Andr=E9?= =?ISO-8859-1?Q?Andr=E9?="
	if hasEncodedWordAhead(str, 24) == -1 {
		t.Error("expecting an encoded word ahead")
	}

}

func TestQueuedID(t *testing.T) {
	h := QueuedID(5550000000, 1)

	if len(h) != 16 { // silly comparison, but there in case of refactoring
		t.Error("queuedID needs to be 16 bytes in length")
	}

	str := h.String()
	if len(str) != 32 {
		t.Error("queuedID string should be 32 bytes in length")
	}

	h2 := QueuedID(5550000000, 1)
	if bytes.Equal(h[:], h2[:]) {
		t.Error("hashes should not be equal")
	}

	h2.FromHex("5a4a2f08784334de5148161943111ad3")
	if h2.String() != "5a4a2f08784334de5148161943111ad3" {
		t.Error("hex conversion didnt work")
	}
}
