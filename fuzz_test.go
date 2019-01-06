// +build gofuzz
package guerrilla

import (
	"testing"

	"io/ioutil"
	"os"
)

// writeCorpos writes data to corpus file name, if it doesn't exists
func writeCorpos(name string, data []byte) {
	if _, err := os.Stat("./workdir/corpus"); err == nil {
		if _, err := os.Stat("./workdir/corpus/" + name); err != nil {
			ioutil.WriteFile("./workdir/corpus/"+name, data, 0644)
		}
	}

}

func TestGenerateCorpus(t *testing.T) {

	str := "EHLO test.com\r\n" +
		"MAIL FROM:<user@example.com>\r\n" +
		"RCPT TO: <test@test.com>" +
		"DATA\r\n" +
		"Subject: Testing Subject\r\n" +
		"\r\n" +
		"..Some body\r\n" +
		"..\r\n" +
		".\r\n"

	writeCorpos("0", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM:user@example.com\r\n" +
		"RCPT TO:<test@test.com>" +
		"DATA\r\n" +
		"Subject: =?ISO-2022-JP?B?GyRCMX5KZxsoQlVSTBskQiROGyhC?=\r\n" +
		"\t=?ISO-2022-JP?B?GyRCJCpDTiRpJDshQxsoQi0xOTYbJEIhbiU5JUglbSVzJTAlPCVtGyhC?=\r\n" +
		"\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("1", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM:<user@example.com>\r\n" +
		"RCPT TO:<test@test.com>\r\n" +
		"RCPT TO:<test2@test.com>\r\n" +
		"RCPT TO:<test3@test.com>\r\n" +
		"DATA\r\n" +
		"Subject: =?ISO-2022-JP?B?GyRCMX5KZxsoQlVSTBskQiROGyhC?=\r\n" +
		"\t=?ISO-2022-JP?B?GyRCJCpDTiRpJDshQxsoQi0xOTYbJEIhbiU5JUglbSVzJTAlPCVtGyhC?=\r\n" +
		"\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("2", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM:<user@example.com> BODY=8BITMIME\r\n" +
		"RCPT TO:<test@test.com>\r\n" +
		"RCPT TO:<test2@test.com>\r\n" +
		"RCPT TO:<test3@test.com>\r\n" +
		"DATA\r\n" +
		"Subject: =?ISO-2022-JP?B?GyRCMX5KZxsoQlVSTBskQiROGyhC?=\r\n" +
		"\t=?ISO-2022-JP?B?GyRCJCpDTiRpJDshQxsoQi0xOTYbJEIhbiU5JUglbSVzJTAlPCVtGyhC?=\r\n" +
		"\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("2", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM:<user@example.com> BODY=8BITMIME\r\n" +
		"HELP\r\n" +
		"NOOP\r\n" +
		"RCPT TO:<test2@test.com>\r\n" +
		"RCPT TO:<test3@test.com>\r\n" +
		"DATA\r\n" +
		"No subject\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("3", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM: <user@example.com> BODY=8BITMIME\r\n" +
		"RSET\r\n" +
		"NOOP\r\n" +
		"RCPT TO:<test2@test.com>\r\n" +
		"RCPT TO:<test3@test.com>\r\n" +
		"DATA\r\n" +
		"No subject\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("4", []byte(str))

	str = "HELO test.com\r\n" + "MAIL FROM:<>\r\n"

	writeCorpos("5", []byte(str))
	str = "MAIL from: <\r\n"
	writeCorpos("6", []byte(str))

	str = "HELO test.com\r\n" + "MAIL FrOm: <<>>\r\n"
	writeCorpos("8", []byte(str))

	str = "HELO test.com\r\n" + "MAIL FrOm:\r\n"
	writeCorpos("7", []byte(str))

	str = "HELO test.com\r\n" + "RCPT TO:\r\n"
	writeCorpos("9", []byte(str))

	str = "HELO test.com\r\n" + "RCPT TO:<>\r\n"
	writeCorpos("10", []byte(str))

	str = "HELO test.com\r\n" + "RCPT TO:<\r\n"
	writeCorpos("11", []byte(str))

	str = "HELO test.com\r\n" + "RCPT TO:<test@test.com> somethingstrange\r\n"
	writeCorpos("12", []byte(str))

	str = "HELO test.com\r\n" + "VRFY\r\n" + "QUIT\r\n"
	writeCorpos("13", []byte(str))

	str = "HELO test.com\r\n" + "VRFY:\r\n"
	writeCorpos("14", []byte(str))

	str = "HELO test.com\r\n" + "VRFY all cows eat grass\r\n"
	writeCorpos("15", []byte(str))

	str = "HELO test.com\r\n" + "RSET\r\n"
	writeCorpos("16", []byte(str))

	str = "HELO test.com\r\n" + "RSET:\r\n"
	writeCorpos("17", []byte(str))

	str = "HELO test.com\r\n" + "RSET all cows eat grass\r\n"
	writeCorpos("18", []byte(str))

	str = "HELO test.com\r\n" + "MAIL FROM: <test@test.com\r\n" +
		"MAIL FROM: <test@test.com\r\n"
	writeCorpos("19", []byte(str))

	str = "HELO test.com\r\n" + "MAIL FROM: <<test@test.com\r\n" +
		"MAIL FROM: <test@test.com\r\n"
	writeCorpos("20", []byte(str))

	str = "HELO test.com\r\n" + "DATA:\r\n"
	writeCorpos("21", []byte(str))

	str = "EHLO test.com\r\n" + "STARTTLS\r\n"
	writeCorpos("22", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM: =?ISO-2022-JP?B?GyRCJEEkUCRSJG0bKEIgbWFnMiAwMDAxMDIzMTIw?= <mailmag@mag2tegami.com>\r\n" +
		"RCPT TO:<test@test.com>\r\n" +
		"DATA\r\n" +
		"Subject: =?ISO-2022-JP?B?GyRCIVokQSRQJFIkbSFbGyhCMy8xMhskQktcRnw9Kk47ISobKEI=?=\r\n" +
		" =?ISO-2022-JP?B?UFBDLUUbJEIhViVeJTklPyE8JTMlcyVGJXMlRCFXGyhC?=\r\n" +
		"\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n"

	writeCorpos("23", []byte(str))

	str = "HELO test.com\r\n" +
		"MAIL FROM:<> BODY=8BITMIME\r\n" +
		"RCPT TO:<test2@test.com>\r\n" +
		"RCPT TO:<test3@test.com>\r\n" +
		"DATA\r\n" +
		"No subject\r\n" +
		"..Now you're just somebody that i used to know\r\n" +
		".\r\n" +
		"QUIT"

	writeCorpos("24", []byte(str))

	str = "HELO test.com\r\n" + "XCLIENT ADDR=127.0.0.1\r\n" + "MAIL FROM:<test@grr.la>\r\n"

	writeCorpos("25", []byte(str))

}

// Tests the Fuzz function.

func TestFuzz(t *testing.T) {
	result := Fuzz([]byte("EHLO me\r\nMail From:test@grr.la\r\nRcpt to:test@test" +
		".com\r\nDATA\r\ntest\r\n.\r\n"))
	if result != 1 {
		t.Error("Fuzz test did not return 1")
	}
	/*
		isFuzzDebug = true
		result = Fuzz([]byte("MAIL from: <\r"))
		if result != 1 {
			t.Error("Fuzz test did not return 1")
		}
		result = Fuzz([]byte("MAIL from: <\r\nHELP\r\n"))
		if result != 1 {
			t.Error("Fuzz test did not return 1")
		}
	*/

}

func TestFuzz2(t *testing.T) {
	isFuzzDebug = true
	result := Fuzz([]byte("MAIL from: <\r\nHELP\r\n"))
	if result != 1 {
		t.Error("Fuzz test did not return 1")
	}

}

func TestFuzz3(t *testing.T) {
	isFuzzDebug = true
	result := Fuzz([]byte("DATA\r\n"))
	if result != 0 {
		t.Error("Fuzz test did not return 1")
	}

}

func TestFuzz4(t *testing.T) {
	isFuzzDebug = true
	result := Fuzz([]byte("MAIL from: <\r"))
	if result != 0 {
		t.Error("Fuzz test did not return 1")
	}

}
