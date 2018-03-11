package mail

import (
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
	if i := strings.Index(str, "用户“Epidemiology in nursing and h  ealth care eBook read/audio id:o  q3jeek”在网站“SPY中文官方网站”  的帐号详情"); i != 0 {
		t.Error("expecting 用户“Epidemiology in nursing and h  ealth care eBook read/audio id:o  q3jeek”在网站“SPY中文官方网站”  的帐号详情, got:", str)
	}
	str = MimeHeaderDecode("=?ISO-8859-1?Q?Andr=E9?= Pirard <PIRARD@vm1.ulg.ac.be>")
	if strings.Index(str, "André Pirard") != 0 {
		t.Error("expecting André Pirard, got:", str)
	}
}
func TestNewAddress(t *testing.T) {

	addr, err := NewAddress("<hoop>")
	if err == nil {
		t.Error("there should be an error:", addr)
	}

	addr, err = NewAddress(`Gogh Fir <tesst@test.com>`)
	if err != nil {
		t.Error("there should be no error:", addr.Host, err)
	}
}
func TestEnvelope(t *testing.T) {
	e := NewEnvelope("127.0.0.1", 22)

	e.QueuedId = "abc123"
	e.Helo = "helo.example.com"
	e.MailFrom = Address{User: "test", Host: "example.com"}
	e.TLS = true
	e.RemoteIP = "222.111.233.121"
	to := Address{User: "test", Host: "example.com"}
	e.PushRcpt(to)
	if to.String() != "test@example.com" {
		t.Error("to does not equal test@example.com, it was:", to.String())
	}
	e.Data.WriteString("Subject: Test\n\nThis is a test nbnb nbnb hgghgh nnnbnb nbnbnb nbnbn.")

	addHead := "Delivered-To: " + to.String() + "\n"
	addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteIP + "])\n"
	e.DeliveryHeader = addHead

	r := e.NewReader()

	data, _ := ioutil.ReadAll(r)
	if len(data) != e.Len() {
		t.Error("e.Len() is inccorrect, it shown ", e.Len(), " but we wanted ", len(data))
	}
	e.ParseHeaders()
	if e.Subject != "Test" {
		t.Error("Subject expecting: Test, got:", e.Subject)
	}

}
