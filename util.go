package guerrilla

import (
	"errors"
	"github.com/flashmob/go-guerrilla/envelope"
	"regexp"
	"strings"
)

var extractEmailRegex, _ = regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!

func extractEmail(str string) (*envelope.EmailAddress, error) {
	email := &envelope.EmailAddress{}
	var err error
	if len(str) > RFC2821LimitPath {
		return email, errors.New("501 Path too long")
	}
	if matched := extractEmailRegex.FindStringSubmatch(str); len(matched) > 2 {
		email.User = matched[1]
		email.Host = validHost(matched[2])
	} else if res := strings.Split(str, "@"); len(res) > 1 {
		email.User = res[0]
		email.Host = validHost(res[1])
	}
	err = nil
	if email.User == "" || email.Host == "" {
		err = errors.New("501 Invalid address")
	} else if len(email.User) > RFC2832LimitLocalPart {
		err = errors.New("501 Local part too long, cannot exceed 64 characters")
	} else if len(email.Host) > RFC2821LimitDomain {
		err = errors.New("501 Domain cannot exceed 255 characters")
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
