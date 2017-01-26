package guerrilla

import (
	"errors"
	"regexp"
	"strings"

	"fmt"

	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/response"
)

var extractEmailRegex, _ = regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!

func extractEmail(str string) (*envelope.EmailAddress, error) {
	email := &envelope.EmailAddress{}
	var err error
	if len(str) > RFC2821LimitPath {
		resp := &response.Response{
			EnhancedCode: response.InvalidCommandArguments,
			BasicCode:    550,
			Class:        response.ClassPermanentFailure,
			Comment:      "Path too long",
		}
		return email, errors.New(resp.String())
	}
	if matched := extractEmailRegex.FindStringSubmatch(str); len(matched) > 2 {
		email.User = matched[1]
		email.Host = validHost(matched[2])
	} else if res := strings.Split(str, "@"); len(res) > 1 {
		email.User = res[0]
		email.Host = validHost(res[1])
	}
	err = nil
	resp := &response.Response{
		EnhancedCode: response.InvalidCommandArguments,
		BasicCode:    501,
		Class:        response.ClassPermanentFailure,
	}
	if email.User == "" || email.Host == "" {
		resp.Comment = "Invalid address"
		err = fmt.Errorf("%s", resp)
	} else if len(email.User) > RFC2832LimitLocalPart {
		resp.BasicCode = 550
		resp.Comment = "Local part too long, cannot exceed 64 characters"
		err = fmt.Errorf("%s", resp)
	} else if len(email.Host) > RFC2821LimitDomain {
		resp.BasicCode = 550
		resp.Comment = "Domain cannot exceed 255 characters"
		err = fmt.Errorf("%s", resp)
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
