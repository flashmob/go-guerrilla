package guerrilla

import (
	"errors"
	"regexp"
	"strings"

	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/response"
)

var extractEmailRegex, _ = regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!

func extractEmail(str string) (mail.Address, error) {
	email := mail.Address{}
	var err error
	if len(str) > RFC2821LimitPath {
		return email, errors.New(response.Canned.FailPathTooLong)
	}
	if matched := extractEmailRegex.FindStringSubmatch(str); len(matched) > 2 {
		email.User = matched[1]
		email.Host = validHost(matched[2])
	} else if res := strings.Split(str, "@"); len(res) > 1 {
		email.User = strings.TrimSpace(res[0])
		email.Host = validHost(res[1])
	}
	err = nil
	if email.User == "" || email.Host == "" {
		err = errors.New(response.Canned.FailInvalidAddress)
	} else if len(email.User) > RFC2832LimitLocalPart {
		err = errors.New(response.Canned.FailLocalPartTooLong)
	} else if len(email.Host) > RFC2821LimitDomain {
		err = errors.New(response.Canned.FailDomainTooLong)
	}
	return email, err
}

var validhostRegex, _ = regexp.Compile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

func validHost(host string) string {
	host = strings.Trim(host, " ")
	if validhostRegex.MatchString(host) {
		return host
	}
	return ""
}
