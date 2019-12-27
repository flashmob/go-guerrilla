package rfc5321

import (
	"strings"
	"testing"
)

func TestParseParam(t *testing.T) {
	s := NewParser([]byte("SIZE=2000"))
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

	s = NewParser([]byte("SI--ZE=2000 BODY=8BITMIME"))
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

	s = NewParser([]byte("SI--ZE-=2000 BODY=8BITMIME")) // illegal - after ZE
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
	if s.LocalPart != "postmaster" {
		t.Error("s.LocalPart should be: postmaster")
	}

	err = s.RcptTo([]byte("<Postmaster@example.com> NOTIFY=SUCCESS,FAILURE"))
	if err != nil {
		t.Error("error not expected ", err)
	}

	err = s.RcptTo([]byte("<\"Postmaster\">"))
	if err != nil {
		t.Error("error not expected ", err)
	}

}

func TestParseForwardPath(t *testing.T) {
	s := NewParser([]byte("<@a,@b:user@[227.0.0.1>")) // missing ]
	err := s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

	s = NewParser([]byte("<@a,@b:user@[527.0.0.1>")) // ip out of range
	err = s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// with a 'size' estmp param
	s = NewParser([]byte("<ned@thor.innosoft.com> NOTIFY=FAILURE ORCPT=rfc822;Carol@Ivory.EDU"))
	err = s.forwardPath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	// tolerate a space at the front
	s = NewParser([]byte(" <ned@thor.innosoft.com>"))
	err = s.forwardPath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	// tolerate a space at the front, invalid
	s = NewParser([]byte(" <"))
	err = s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// tolerate a space at the front, invalid
	s = NewParser([]byte(" "))
	err = s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// empty
	s = NewParser([]byte(""))
	err = s.forwardPath()
	if err == nil {
		t.Error("error expected ", err)
	}

}

func TestParseReversePath(t *testing.T) {

	s := NewParser([]byte("<@a,@b:user@d>"))
	err := s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("<@a,@b:user@d> param=some-value")) // includes a mail parameter
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("<@a,@b:user@[227.0.0.1]>"))
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("<>"))
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte(""))
	err = s.reversePath()
	if err == nil {
		t.Error("error  expected ", err)
	}

	s = NewParser([]byte("test@rcample.com"))
	err = s.reversePath()
	if err == nil {
		t.Error("error expected ", err)
	}

	s = NewParser([]byte("<@ghg;$7@65"))
	err = s.reversePath()
	if err == nil {
		t.Error("error  expected ", err)
	}

	// tolerate a space at the front
	s = NewParser([]byte(" <>"))
	err = s.reversePath()
	if err != nil {
		t.Error("error not expected ", err)
	}

	// tolerate a space at the front, invalid
	s = NewParser([]byte(" <"))
	err = s.reversePath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// tolerate a space at the front, invalid
	s = NewParser([]byte(" "))
	err = s.reversePath()
	if err == nil {
		t.Error("error expected ", err)
	}

	// empty
	s = NewParser([]byte(" "))
	err = s.reversePath()
	if err == nil {
		t.Error("error expected ", err)
	}
}

func TestParseIpv6Address(t *testing.T) {
	s := NewParser([]byte("2001:0000:3238:DFE1:0063:0000:0000:FEFB"))
	err := s.ipv6AddressLiteral()
	if s.accept.String() != "2001:0:3238:dfe1:63::fefb" {
		t.Error("expected 2001:0:3238:dfe1:63::fefb, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}
	s = NewParser([]byte("2001:3238:DFE1:6323:FEFB:2536:1.2.3.2"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "2001:3238:dfe1:6323:fefb:2536:102:302" {
		t.Error("expected 2001:3238:dfe1:6323:fefb:2536:102:302, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("2001:0000:3238:DFE1:63:0000:0000:FEFB"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "2001:0:3238:dfe1:63::fefb" {
		t.Error("expected 2001:0:3238:dfe1:63::fefb, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("2001:0000:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "2001:0:3238:dfe1:63::fefb" {
		t.Error("expected 2001:0:3238:dfe1:63::fefb, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("2001:0:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "2001:0:3238:dfe1:63::fefb" {
		t.Error("expected 2001:0:3238:dfe1:63::fefb, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("g001:0:3238:DFE1:63::FEFB"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "" {
		t.Error("expected \"\", got:", s.accept.String())
	}
	if err == nil {
		t.Error("error expected ", err)
	}

	s = NewParser([]byte("g001:0:3238:DFE1::63::FEFB"))
	err = s.ipv6AddressLiteral()
	if s.accept.String() != "" {
		t.Error("expected \"\", got:", s.accept.String())
	}
	if err == nil {
		t.Error("error expected ", err)
	}
}

func TestParseIpv4Address(t *testing.T) {
	s := NewParser([]byte("0.0.0.255"))
	err := s.ipv4AddressLiteral()
	if s.accept.String() != "0.0.0.255" {
		t.Error("expected 0.0.0.255, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ", err)
	}

	s = NewParser([]byte("0.0.0.256"))
	err = s.ipv4AddressLiteral()
	if s.accept.String() != "0.0.0.256" {
		t.Error("expected 0.0.0.256, got:", s.accept.String())
	}
	if err == nil {
		t.Error("error expected ", err)
	}

}

func TestParseMailBoxBad(t *testing.T) {

	// must be quoted
	s := NewParser([]byte("Abc\\@def@example.com"))
	err := s.mailbox()

	if err == nil {
		t.Error("error expected")
	}

	// must be quoted
	s = NewParser([]byte("Fred\\ Bloggs@example.com"))
	err = s.mailbox()

	if err == nil {
		t.Error("error expected")
	}
}

func TestParseMailbox(t *testing.T) {

	s := NewParser([]byte("jsmith@[IPv6:2001:db8::1]"))
	err := s.mailbox()
	if s.Domain != "2001:db8::1" {
		t.Error("expected domain: 2001:db8::1, got:", s.Domain)
	}
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("\"qu\\{oted\"@test.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("\"qu\\{oted\"@[127.0.0.1]"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("jsmith@[IPv6:2001:db8::1]"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("Joe.\\Blow@example.com"))
	err = s.mailbox()
	if err == nil {
		t.Error("error expected ")
	}
	s = NewParser([]byte("\"Abc@def\"@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("\"Fred Bloggs\"@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("customer/department=shipping@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("$A12345@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("!def!xyz%abc@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("_somename@example.com"))
	err = s.mailbox()
	if err != nil {
		t.Error("error not expected ")
	}

}

func TestParseLocalPart(t *testing.T) {
	s := NewParser([]byte("\"qu\\{oted\""))
	err := s.localPart()
	if s.LocalPart != "qu{oted" {
		t.Error("expected qu\\{oted, got:", s.LocalPart)
	}
	if s.LocalPartQuotes == true {
		t.Error("local part does not need to be quoted")
	}
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("dot.string"))
	err = s.localPart()
	if s.LocalPart != "dot.string" {
		t.Error("expected dot.string, got:", s.LocalPart)
	}
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("dot.st!ring"))
	err = s.localPart()
	if s.LocalPart != "dot.st!ring" {
		t.Error("expected dot.st!ring, got:", s.LocalPart)
	}
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("dot..st!ring")) // fail
	err = s.localPart()

	if err == nil {
		t.Error("error expected ")
	}
}

func TestParseQuotedString(t *testing.T) {
	s := NewParser([]byte("\"qu\\ oted\""))
	err := s.quotedString()
	if s.accept.String() != "qu oted" {
		t.Error("Expected qu oted, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("\"@\""))
	err = s.quotedString()
	if s.accept.String() != "@" {
		t.Error("Expected @, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParseDotString(t *testing.T) {

	s := NewParser([]byte("Joe..Blow"))
	err := s.dotString()
	if err == nil {
		t.Error("error expected ")
	}

	s = NewParser([]byte("Joe.Blow"))
	err = s.dotString()
	if s.accept.String() != "Joe.Blow" {
		t.Error("Expected Joe.Blow, got:", s.accept.String())
	}
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParsePath(t *testing.T) {
	s := NewParser([]byte("<foo>")) // requires @
	err := s.path()
	if err == nil {
		t.Error("error expected ")
	}
	s = NewParser([]byte("<@example.com,@test.com:foo@example.com>"))
	err = s.path()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("<@example.com>")) // no mailbox
	err = s.path()
	if err == nil {
		t.Error("error expected ")
	}

	s = NewParser([]byte("<test@example.com	1")) // no closing >
	err = s.path()
	if err == nil {
		t.Error("error expected ")
	}
}

func TestParseADL(t *testing.T) {
	s := NewParser([]byte("@example.com,@test.com"))
	err := s.adl()
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParseAtDomain(t *testing.T) {
	s := NewParser([]byte("@example.com"))
	err := s.atDomain()
	if err != nil {
		t.Error("error not expected ")
	}
}

func TestParseDomain(t *testing.T) {

	s := NewParser([]byte("a"))
	err := s.domain()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("a^m.com"))
	err = s.domain()
	if err == nil {
		t.Error("error was expected ")
	}

	s = NewParser([]byte("a.com.gov"))
	err = s.domain()
	if err != nil {
		t.Error("error not expected ")
	}

	s = NewParser([]byte("wrong-.com"))
	err = s.domain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParser([]byte("wrong."))
	err = s.domain()
	if err == nil {
		t.Error("error was expected ")
	}
}

func TestParseSubDomain(t *testing.T) {

	s := NewParser([]byte("a"))
	err := s.subdomain()
	if err != nil {
		t.Error("error not expected ")
	}
	s = NewParser([]byte("-a"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParser([]byte("a--"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParser([]byte("a--"))
	err = s.subdomain()
	if err == nil {
		t.Error("error was expected ")
	}
	s = NewParser([]byte("a--b"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	// although a---b looks like an illegal subdomain, it is rfc5321 grammar spec
	s = NewParser([]byte("a---b"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	s = NewParser([]byte("abc"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

	s = NewParser([]byte("a-b-c"))
	err = s.subdomain()
	if err != nil {
		t.Error("error was not expected ")
	}

}

func TestPostmasterQuoted(t *testing.T) {

	var s Parser
	err := s.RcptTo([]byte("<\"Po\\stmas\\ter\">"))
	if err != nil {
		t.Error("error not expected ", err)
	}
}

func TestParse(t *testing.T) {

	s := NewParser([]byte("<"))
	err := s.reversePath()
	if err == nil {
		t.Error("< expected parse error")
	}

	// the @ needs to be quoted
	s = NewParser([]byte("<@m.conm@test.com>"))
	err = s.reversePath()
	if err == nil {
		t.Error("expected parse error", err)
	}

	s = NewParser([]byte("<\"@m.conm\"@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error", err)
	}

	s = NewParser([]byte("<m-m.conm@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}

	s = NewParser([]byte("<@test:user@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}
	s = NewParser([]byte("<@test,@test2:user@test.com>"))
	err = s.reversePath()
	if err != nil {
		t.Error("not expected parse error")
	}

}

func TestEhlo(t *testing.T) {
	var s Parser
	domain, ip, err := s.Ehlo([]byte(" hello.com"))
	if ip != nil {
		t.Error("ip should be nil")
	}
	if err != nil {
		t.Error(err)
	}
	if domain != "hello.com" {
		t.Error("domain not hello.com")
	}

	domain, ip, err = s.Ehlo([]byte(" [211.0.0.3]"))
	if err != nil {
		t.Error(err)
	}
	if ip == nil {
		t.Error("ip should not be nil")
	}

	if domain != "211.0.0.3" {
		t.Error("expecting domain to be 211.0.0.3")
	}
}

func TestHelo(t *testing.T) {
	var s Parser
	domain, err := s.Helo([]byte(" example.com"))
	if err != nil {
		t.Error(err)
	}
	if domain != "example.com" {
		t.Error("expecting domain = example.com")
	}

	domain, err = s.Helo([]byte(" exam_ple.com"))
	if err == nil {
		t.Error("expecting domain exam_ple.com to be invalid")
	}
}
