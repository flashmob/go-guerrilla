package rfc5321

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseParam(t *testing.T) {
	s := NewParserFromBytes([]byte("SIZE=2000"))
	params, err := s.param()
	if strings.Compare(params[0], "SIZE") != 0 {
		t.Error("SIZE ecpected")
	}
	if strings.Compare(params[1], "2000") != 0 {
		t.Error("2000 ecpected")
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("SI--ZE=2000 BODY=8BITMIME"))
	tup, err := s.parameters()
	if strings.Compare(tup[0][0], "SI--ZE") != 0 {
		t.Error("SI--ZE ecpected")
	}
	if strings.Compare(tup[0][1], "2000") != 0 {
		t.Error("2000 ecpected")
	}
	if strings.Compare(tup[1][0], "BODY") != 0 {
		t.Error("BODY expected", err)
	}
	if strings.Compare(tup[1][1], "8BITMIME") != 0 {
		t.Error("8BITMIME expected", err)
	}

	s = NewParserFromBytes([]byte("SI--ZE-=2000 BODY=8BITMIME")) // illegal - after ZE
	tup, err = s.parameters()
	if err == nil {
		t.Error("error was expected ")
	}
}

func TestParseRcptTo(t *testing.T) {
	var s Parser
	err := s.RcptTo([]byte("<Postmaster>"))
	if err != nil {
		t.Error("error not expected ", err)
	}

	err = s.RcptTo([]byte("<Postmaster@example.com>"))
	if err != nil {
		t.Error("error not expected ", err)
	}
	if s.LocalPart != "Postmaster" {
		t.Error("s.LocalPart should be: Postmaster")
	}

	err = s.RcptTo([]byte("<Postmaster@example.com> NOTIFY=SUCCESS,FAILURE"))
	if err != nil {
		t.Error("error not expected ", err)
	}

	//
}

func TestParseForwardPath(t *testing.T) {
	s := NewParserFromBytes([]byte("<@a,@b:user@[227.0.0.1>")) // missing ]
	err := s.forwardPath()
	if err == nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("<@a,@b:user@[527.0.0.1>")) // ip out of range
	err = s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// with a 'size' estmp param
	s = NewParserFromBytes([]byte("<ned@thor.innosoft.com> NOTIFY=FAILURE ORCPT=rfc822;Carol@Ivory.EDU"))
	err = s.forwardPath()
	if err != nil {
		t.Error("error expected ", err)
	}

}

func TestParseReversePath(t *testing.T) {

	s := NewParserFromBytes([]byte("<@a,@b:user@d>"))
	err := s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("<@a,@b:user@d> param=some-value")) // includes a mail parameter
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("<@a,@b:user@[227.0.0.1]>"))
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("<>"))
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte(""))
	err = s.reversePath()
	if err == nil {
		t.Error("error  expected ", err)
	}

	s = NewParserFromBytes([]byte("test@rcample.com"))
	err = s.reversePath()
	if err == nil {
		t.Error("error  expected ", err)
	}

	s = NewParserFromBytes([]byte("<@ghg;$7@65"))
	err = s.reversePath()
	if err == nil {
		t.Error("error  expected ", err)
	}
}

func TestParseIpv6Address(t *testing.T) {
	s := NewParserFromBytes([]byte("2001:0000:3238:DFE1:0063:0000:0000:FEFB"))
	err := s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}
	s = NewParserFromBytes([]byte("2001:3238:DFE1:6323:FEFB:2536:1.2.3.2"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("2001:0000:3238:DFE1:63:0000:0000:FEFB"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("2001:0000:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("2001:0:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("g001:0:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err == nil {
		t.Error("error expected ", err)
	}

	s = NewParserFromBytes([]byte("g001:0:3238:DFE1::63::FEFB"))
	err = s.ipv6AddressLiteral()
	fmt.Println(s.accept.String())
	if err == nil {
		t.Error("error expected ", err)
	}
}

func TestParseIpv4Address(t *testing.T) {
	s := NewParserFromBytes([]byte("0.0.0.255"))
	err := s.ipv4AddressLiteral()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParserFromBytes([]byte("0.0.0.256"))
	err = s.ipv4AddressLiteral()
	fmt.Println(s.accept.String())
	if err == nil {
		t.Error("error expected ", err)
	}

}

func TestParseMailBoxBad(t *testing.T) {

	// must be quoted
	s := NewParserFromBytes([]byte("Abc\\@def@example.com"))
	err := s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err == nil {
		t.Error("error expected ")
	}

	// must be quoted
	s = NewParserFromBytes([]byte("Fred\\ Bloggs@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err == nil {
		t.Error("error expected ")
	}
}

func TestParseMailbox(t *testing.T) {

	s := NewParserFromBytes([]byte("jsmith@[IPv6:2001:db8::1]"))
	err := s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("\"qu\\{oted\"@test.com"))
	err = s.mailbox()
	fmt.Println(s.LocalPart)
	fmt.Println(s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("\"qu\\{oted\"@[127.0.0.1]"))
	err = s.mailbox()
	fmt.Println(s.LocalPart)
	fmt.Println(s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("jsmith@[IPv6:2001:db8::1]"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("Joe.\\Blow@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("\"Abc@def\"@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("\"Fred Bloggs\"@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("customer/department=shipping@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("$A12345@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("!def!xyz%abc@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("_somename@example.com"))
	err = s.mailbox()
	fmt.Println("lp:", s.LocalPart)
	fmt.Println("d", s.Domain)
	if err != nil {
		t.Error("error not expected ")
	}

}

func TestParseLocalPart(t *testing.T) {
	s := NewParserFromBytes([]byte("\"qu\\{oted\""))
	err := s.localPart()
	fmt.Println(s.LocalPart)
	if err != nil {
		t.Error("error expected ")
	}
	s = NewParserFromBytes([]byte("dot.string"))
	err = s.localPart()
	fmt.Println(s.LocalPart)
	if err != nil {
		t.Error("error expected ")
	}
	s = NewParserFromBytes([]byte("dot.st!ring"))
	err = s.localPart()
	fmt.Println(s.LocalPart)
	if err != nil {
		t.Error("error expected ")
	}
	s = NewParserFromBytes([]byte("dot..st!ring")) // fail
	err = s.localPart()
	fmt.Println(s.LocalPart)
	if err == nil {
		t.Error("error expected ")
	}
}

func TestParseQuotedString(t *testing.T) {
	s := NewParserFromBytes([]byte("\"qu\\ oted\""))
	err := s.quotedString()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error expected ")
	}

	s = NewParserFromBytes([]byte("\"@\""))
	err = s.quotedString()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error expected ")
	}
}

func TestParseDotString(t *testing.T) {

	s := NewParserFromBytes([]byte("Joe..\\\\Blow"))
	err := s.dotString()
	fmt.Println(s.accept.String())
	if err == nil {
		t.Error("error expected ")
	}

	s = NewParserFromBytes([]byte("Joe.\\\\Blow"))
	err = s.dotString()
	fmt.Println(s.accept.String())
	if err != nil {
		t.Error("error expected ")
	}
}

func TestParsePath(t *testing.T) {
	s := NewParserFromBytes([]byte("<foo>")) // requires @
	err := s.path()
	if err == nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("<@example.com,@test.com:foo@example.com>"))
	err = s.path()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("<@example.com>")) // no mailbox
	err = s.path()
	if err == nil {
		t.Error("error expected ")
	}

	s = NewParserFromBytes([]byte("<test@example.com	1")) // no closing >
	err = s.path()
	if err == nil {
		t.Error("error expected ")
	}
}

func TestParseADL(t *testing.T) {
	s := NewParserFromBytes([]byte("@example.com,@test.com"))
	err := s.adl()
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParseAtDomain(t *testing.T) {
	s := NewParserFromBytes([]byte("@example.com"))
	err := s.atDomain()
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParseDomain(t *testing.T) {

	s := NewParserFromBytes([]byte("a"))
	err := s.domain()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("a.com.gov"))
	err = s.domain()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParserFromBytes([]byte("wrong-.com"))
	err = s.domain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParserFromBytes([]byte("wrong."))
	err = s.domain()
	if err == nil {
		t.Error("error was expected ")
	}
}

func TestParseSubDomain(t *testing.T) {
	s := NewParserFromBytes([]byte("a"))
	err := s.subdomain()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParserFromBytes([]byte("-a"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParserFromBytes([]byte("a--"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParserFromBytes([]byte("a--"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParserFromBytes([]byte("a--b"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	// although a---b looks like an illegal subdomain, it is rfc5321 grammar spec
	s = NewParserFromBytes([]byte("a---b"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	s = NewParserFromBytes([]byte("abc"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	s = NewParserFromBytes([]byte("a-b-c"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

}
func TestParse(t *testing.T) {

	s := NewParserFromBytes([]byte("<"))
	err := s.reversePath()
	if err == nil {
		t.Error("< expected parse error")
	}

	// the @ needs to be quoted
	s = NewParserFromBytes([]byte("<@m.conm@test.com>"))
	err = s.reversePath()
	if err == nil {
		t.Error("expected parse error", err)
	}

	s = NewParserFromBytes([]byte("<\"@m.conm\"@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error", err)
	}

	s = NewParserFromBytes([]byte("<m-m.conm@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}

	s = NewParserFromBytes([]byte("<@test:user@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}
	s = NewParserFromBytes([]byte("<@test,@test2:user@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}

}
