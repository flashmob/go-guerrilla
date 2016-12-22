package guerrilla

import (
	"errors"
	"regexp"
	"strings"
)

var extractEmailRegex, _ = regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!

func extractEmail(str string) (*EmailAddress, error) {
	email := &EmailAddress{}
	var err error
	if matched := extractEmailRegex.FindStringSubmatch(str); len(matched) > 2 {
		email.User = matched[1]
		email.Host = validHost(matched[2])
	} else if res := strings.Split(str, "@"); len(res) > 1 {
		email.User = res[0]
		email.Host = validHost(res[1])
	}
	if email.User == "" || email.Host == "" {
		err = errors.New("Invalid address, [" + email.User + "@" + email.Host + "] address:" + str)
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
