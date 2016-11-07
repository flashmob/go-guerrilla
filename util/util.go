package util

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"

	"gopkg.in/iconv.v1"

	"github.com/sloonz/go-qprintable"

	guerrilla "github.com/flashmob/go-guerrilla"
)

var allowedHosts map[string]bool

// map the allow hosts for easy lookup
func prepareAllowedHosts(allowedHostsStr string) {
	allowedHosts = make(map[string]bool, 15)
	if arr := strings.Split(allowedHostsStr, ","); len(arr) > 0 {
		for i := 0; i < len(arr); i++ {
			allowedHosts[arr[i]] = true
		}
	}
}

// TODO: cleanup
func ValidateEmailData(client *guerrilla.Client, allowedHostsStr string) (user string, host string, addr_err error) {
	if allowedHosts == nil {
		prepareAllowedHosts(allowedHostsStr)
	}

	if user, host, addr_err = extractEmail(client.MailFrom); addr_err != nil {
		return user, host, addr_err
	}
	client.MailFrom = user + "@" + host
	if user, host, addr_err = extractEmail(client.RcptTo); addr_err != nil {
		return user, host, addr_err
	}
	client.RcptTo = user + "@" + host
	// check if on allowed hosts
	if allowed := allowedHosts[strings.ToLower(host)]; !allowed {
		return user, host, errors.New("invalid host:" + host)
	}
	return user, host, addr_err
}

var extractEmailRegex, _ = regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!

func extractEmail(str string) (name string, host string, err error) {
	if matched := extractEmailRegex.FindStringSubmatch(str); len(matched) > 2 {
		host = validHost(matched[2])
		name = matched[1]
	} else {
		if res := strings.Split(str, "@"); len(res) > 1 {
			name = res[0]
			host = validHost(res[1])
		}
	}
	if host == "" || name == "" {
		err = errors.New("Invalid address, [" + name + "@" + host + "] address:" + str)
	}
	return name, host, err
}

var mimeRegex, _ = regexp.Compile(`=\?(.+?)\?([QBqp])\?(.+?)\?=`)

// Decode strings in Mime header format
// eg. =?ISO-2022-JP?B?GyRCIVo9dztSOWJAOCVBJWMbKEI=?=
func MimeHeaderDecode(str string) string {

	matched := mimeRegex.FindAllStringSubmatch(str, -1)
	var charset, encoding, payload string
	if matched != nil {
		for i := 0; i < len(matched); i++ {
			if len(matched[i]) > 2 {
				charset = matched[i][1]
				encoding = strings.ToUpper(matched[i][2])
				payload = matched[i][3]
				switch encoding {
				case "B":
					str = strings.Replace(
						str,
						matched[i][0],
						MailTransportDecode(payload, "base64", charset),
						1)
				case "Q":
					str = strings.Replace(
						str,
						matched[i][0],
						MailTransportDecode(payload, "quoted-printable", charset),
						1)
				}
			}
		}
	}
	return str
}

var valihostRegex, _ = regexp.Compile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

func validHost(host string) string {
	host = strings.Trim(host, " ")
	if valihostRegex.MatchString(host) {
		return host
	}
	return ""
}

// decode from 7bit to 8bit UTF-8
// encodingType can be "base64" or "quoted-printable"
func MailTransportDecode(str string, encodingType string, charset string) string {
	if charset == "" {
		charset = "UTF-8"
	} else {
		charset = strings.ToUpper(charset)
	}
	if encodingType == "base64" {
		str = fromBase64(str)
	} else if encodingType == "quoted-printable" {
		str = fromQuotedP(str)
	}

	if charset != "UTF-8" {
		charset = fixCharset(charset)
		if cd, err := iconv.Open("UTF-8", charset); err == nil {
			defer func() {
				cd.Close()
				if r := recover(); r != nil {
					//logln(1, fmt.Sprintf("Recovered in %v", r))
				}
			}()
			// eg. charset can be "ISO-2022-JP"
			return cd.ConvString(str)
		}

	}
	return str
}

func fromBase64(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := base64.NewDecoder(base64.StdEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

func fromQuotedP(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := qprintable.NewDecoder(qprintable.BinaryEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

var charsetRegex, _ = regexp.Compile(`[_:.\/\\]`)

func fixCharset(charset string) string {
	fixed_charset := charsetRegex.ReplaceAllString(charset, "-")
	// Fix charset
	// borrowed from http://squirrelmail.svn.sourceforge.net/viewvc/squirrelmail/trunk/squirrelmail/include/languages.php?revision=13765&view=markup
	// OE ks_c_5601_1987 > cp949
	fixed_charset = strings.Replace(fixed_charset, "ks-c-5601-1987", "cp949", -1)
	// Moz x-euc-tw > euc-tw
	fixed_charset = strings.Replace(fixed_charset, "x-euc", "euc", -1)
	// Moz x-windows-949 > cp949
	fixed_charset = strings.Replace(fixed_charset, "x-windows_", "cp", -1)
	// windows-125x and cp125x charsets
	fixed_charset = strings.Replace(fixed_charset, "windows-", "cp", -1)
	// ibm > cp
	fixed_charset = strings.Replace(fixed_charset, "ibm", "cp", -1)
	// iso-8859-8-i -> iso-8859-8
	fixed_charset = strings.Replace(fixed_charset, "iso-8859-8-i", "iso-8859-8", -1)
	if charset != fixed_charset {
		return fixed_charset
	}
	return charset
}

// returns an md5 hash as string of hex characters
func MD5Hex(stringArguments ...*string) string {
	h := md5.New()
	var r *strings.Reader
	for i := 0; i < len(stringArguments); i++ {
		r = strings.NewReader(*stringArguments[i])
		io.Copy(h, r)
	}
	sum := h.Sum([]byte{})
	return fmt.Sprintf("%x", sum)
}

// concatenate & compress all strings  passed in
func Compress(stringArguments ...*string) string {
	var b bytes.Buffer
	var r *strings.Reader
	w, _ := zlib.NewWriterLevel(&b, zlib.BestSpeed)
	for i := 0; i < len(stringArguments); i++ {
		r = strings.NewReader(*stringArguments[i])
		io.Copy(w, r)
	}
	w.Close()
	return b.String()
}
