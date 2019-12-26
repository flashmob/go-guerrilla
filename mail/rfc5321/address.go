package rfc5321

import (
	"errors"
	"net"
)

// Parse productions according to ABNF in RFC5322
type RFC5322 struct {
	AddressList
	Parser
	addr SingleAddress
}

type AddressList struct {
	List  []SingleAddress
	Group string
}

type SingleAddress struct {
	DisplayName       string
	DisplayNameQuoted bool
	LocalPart         string
	LocalPartQuoted   bool
	Domain            string
	IP                net.IP
	NullPath          bool
}

var (
	errNotAtom               = errors.New("not atom")
	errExpectingAngleAddress = errors.New("not angle address")
	errNotAWord              = errors.New("not a word")
	errExpectingColon        = errors.New("expecting : ")
	errExpectingSemicolon    = errors.New("expecting ; ")
	errExpectingAngleClose   = errors.New("expecting >")
	errExpectingAngleOpen    = errors.New("< expected")
	errQuotedUnclosed        = errors.New("quoted string not closed")
)

// Address parses the "address" production specified in RFC5322
// address         =   mailbox / group
func (s *RFC5322) Address(input []byte) (AddressList, error) {
	s.set(input)
	s.next()
	s.List = nil
	s.addr = SingleAddress{}
	if err := s.mailbox(); err != nil {
		if s.ch == ':' {
			if groupErr := s.group(); groupErr != nil {
				return s.AddressList, groupErr
			} else {
				err = nil
			}
		}
		return s.AddressList, err

	}
	return s.AddressList, nil
}

// group  =  display-name ":" [group-List] ";" [CFWS]
func (s *RFC5322) group() error {
	if s.addr.DisplayName == "" {
		if err := s.displayName(); err != nil {
			return err
		}
	} else {
		s.Group = s.addr.DisplayName
		s.addr.DisplayName = ""
	}
	if s.ch != ':' {
		return errExpectingColon
	}
	s.next()
	_ = s.groupList()
	s.skipSpace()
	if s.ch != ';' {
		return errExpectingSemicolon
	}
	return nil
}

// mailbox  =   name-addr / addr-spec
func (s *RFC5322) mailbox() error {
	pos := s.pos // save the position
	if err := s.nameAddr(); err != nil {
		if err == errExpectingAngleAddress && s.ch != ':' { // ':' means it's a group
			// we'll attempt to parse as an email address without angle brackets
			s.addr.DisplayName = ""
			s.addr.DisplayNameQuoted = false
			s.pos = pos - 1 //- 1 // rewind to the saved position
			if s.pos > -1 {
				s.ch = s.buf[s.pos]
			}
			if err = s.Parser.mailbox(); err != nil {
				return err
			}
			s.addAddress()
		} else {
			return err
		}
	}
	return nil
}

// addAddress ads the current address to the List
func (s *RFC5322) addAddress() {
	s.addr.LocalPart = s.LocalPart
	s.addr.LocalPartQuoted = s.LocalPartQuotes
	s.addr.Domain = s.Domain
	s.addr.IP = s.IP
	s.List = append(s.List, s.addr)
	s.addr = SingleAddress{}
}

// nameAddr consumes the name-addr production.
// name-addr =  [display-name] angle-addr
func (s *RFC5322) nameAddr() error {
	_ = s.displayName()
	if s.ch == '<' {
		if err := s.angleAddr(); err != nil {
			return err
		}
		s.next()
		if s.ch != '>' {
			return errExpectingAngleClose
		}
		s.addAddress()
		return nil
	} else {
		return errExpectingAngleAddress
	}

}

// angleAddr consumes the angle-addr production
// angle-addr      =   [CFWS] "<" addr-spec ">" [CFWS] / obs-angle-addr
func (s *RFC5322) angleAddr() error {
	s.skipSpace()
	if s.ch != '<' {
		return errExpectingAngleOpen
	}
	// addr-spec       =   local-part "@" domain
	if err := s.Parser.mailbox(); err != nil {
		return err
	}
	s.skipSpace()
	return nil
}

// displayName consumes the display-name production:
// display-name    =   phrase
// phrase          =   1*word / obs-phrase
func (s *RFC5322) displayName() error {
	defer func() {
		if s.accept.Len() > 0 {
			s.addr.DisplayName = s.accept.String()
			s.accept.Reset()
		}
	}()
	// phrase
	if err := s.word(); err != nil {
		return err
	}
	for {
		err := s.word()
		if err != nil {
			return nil
		}
	}
}

// quotedString consumes a quoted-string production
func (s *RFC5322) quotedString() error {
	if s.ch == '"' {
		if err := s.Parser.QcontentSMTP(); err != nil {
			return err
		}
		if s.ch != '"' {
			return errQuotedUnclosed
		} else {
			// accept the "
			s.next()
		}
	}
	return nil
}

// word = atom / quoted-string
func (s *RFC5322) word() error {
	if s.ch == '"' {
		s.addr.DisplayNameQuoted = true
		return s.quotedString()
	} else if s.isAtext(s.ch) || s.ch == ' ' || s.ch == '\t' {
		return s.atom()
	}
	return errNotAWord
}

// atom = [CFWS] 1*atext [CFWS]
func (s *RFC5322) atom() error {
	s.skipSpace()
	if !s.isAtext(s.ch) {
		return errNotAtom
	}
	for {
		if s.isAtext(s.ch) {
			s.accept.WriteByte(s.ch)
			s.next()
		} else {
			skipped := s.skipSpace()
			if !s.isAtext(s.ch) {
				return nil
			}
			if skipped > 0 {
				s.accept.WriteByte(' ')
			}
			s.accept.WriteByte(s.ch)
			s.next()
		}
	}
}

// groupList consumes the "group-List" production:
// group-List      =   mailbox-List / CFWS / obs-group-List
func (s *RFC5322) groupList() error {
	// mailbox-list    =   (mailbox *("," mailbox))
	if err := s.mailbox(); err != nil {
		return err
	}
	s.next()
	for {
		s.skipSpace()
		if s.ch != ',' {
			return nil
		}
		s.next()
		s.skipSpace()
		if err := s.mailbox(); err != nil {
			return err
		}
		s.next()
	}
}

// skipSpace skips vertical space by calling next(), returning the count of spaces skipped
func (s *RFC5322) skipSpace() int {
	var skipped int
	for {
		if s.ch != ' ' && s.ch != 9 {
			return skipped
		}
		s.next()
		skipped++
	}
}
