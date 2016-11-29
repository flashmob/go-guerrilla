package main

import (
	"fmt"
	"net/smtp"
)

const (
	URL = "127.0.0.1:2500"
)

func lastWords(message string, err error) {
	fmt.Println(message, err.Error())
	panic(err)
}

func main() {
	c, err := smtp.Dial(URL)
	if err != nil {
		lastWords("Dial ", err)
	}

	from := "jordan.schalm@gmail.com"
	to := "somebody.else@gmail.com"

	if err = c.Mail(from); err != nil {
		lastWords("Mail ", err)
	}

	if err = c.Rcpt(to); err != nil {
		lastWords("Rcpt ", err)
	}

	wr, err := c.Data()
	if err != nil {
		lastWords("Data ", err)
	}
	defer wr.Close()

	msg := fmt.Sprint("Subject: something\n")
	msg += "From: " + from + "\n"
	msg += "To: " + to + "\n"
	msg += "\n\n"
	msg += "hello\n"

	_, err = fmt.Fprint(wr, msg)
	if err != nil {
		lastWords("Send ", err)
	}

	err = c.Quit()
	if err != nil {
		lastWords("Quit ", err)
	}
}
