package rfc5321

import (
	"testing"
)

func TestParseRFC5322(t *testing.T) {
	var s RFC5322
	if _, err := s.Address([]byte("\"Mike Jones\" <test@tdomain.com>")); err != nil {
		t.Error(err)
	}
	// parse a simple address
	if a, err := s.Address([]byte("test@tdomain.com")); err != nil {
		t.Error(err)
	} else {
		if len(a.List) != 1 {
			t.Error("expecting 1 address")
		} else {
			// display name should be empty
		}
	}
}

func TestParseRFC5322Decoder(t *testing.T) {
	var s RFC5322
	if _, err := s.Address([]byte("=?ISO-8859-1?Q?Andr=E9?= =?ISO-8859-1?Q?Andr=E9?= <test@tdomain.com>")); err != nil {
		t.Error(err)
	}
}

func TestParseRFC5322IP(t *testing.T) {
	var s RFC5322
	// this is an incorrect IPv6 address
	if _, err := s.Address([]byte("\"Mike Jones\" <\"testing 123\"@[IPv6:IPv6:2001:db8::1]>")); err == nil {
		t.Error("Expecting error, because Ip address was wrong")
	}
	// this one is correct, with quoted display name and quoted local-part
	if a, err := s.Address([]byte("\"Mike Jones\" <\"testing 123\"@[IPv6:2001:db8::1]>")); err != nil {
		t.Error(err)
	} else {
		if len(a.List) != 1 {
			t.Error("expecting 1 address, but got", len(a.List))
		} else {
			if a.List[0].DisplayNameQuoted == false {
				t.Error(".List[0].DisplayNameQuoted is false, expecting true")
			}
			if a.List[0].LocalPartQuoted == false {
				t.Error(".List[0].LocalPartQuotes is false, expecting true")
			}
			if a.List[0].IP == nil {
				t.Error("a.List[0].IP should not be nil")
			}
			if a.List[0].Domain != "2001:db8::1" {
				t.Error("a.List[0].Domain should be, but got", a.List[0].Domain)
			}
		}
	}
}

func TestParseRFC5322Group(t *testing.T) {
	// A Group:Ed Jones <c@a.test>,joe@where.test,John <jdoe@one.test>;
	var s RFC5322
	if a, err := s.Address([]byte("A Group:Ed Jones <c@a.test>,joe@where.test,John <jdoe@one.test> , \"te \\\" st\"<test@example.com> ;")); err != nil {
		t.Error(err)
	} else {
		if a.Group != "A Group" {
			t.Error("expecting a.Group to be \"A Group\" but got:", a.Group)
		}
		if len(a.List) != 4 {
			t.Error("expecting 4 addresses, but got", len(a.List))
		} else {
			if a.List[0].DisplayName != "Ed Jones" {
				t.Error("expecting a.List[0].DisplayName 'Ed Jones' but got:", a.List[0].DisplayName)
			}
			if a.List[0].LocalPart != "c" {
				t.Error("expecting a.List[0].LocalPart 'c' but got:", a.List[0].LocalPart)
			}
			if a.List[0].Domain != "a.test" {
				t.Error("expecting a.List[0].Domain 'a.test' but got:", a.List[0].Domain)
			}
		}

	}
}
