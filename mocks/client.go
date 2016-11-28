package main

import (
	"fmt"
	"net/smtp"
)

const (
	URL = "127.0.0.1:2500"
)

func main() {
	c, err := smtp.Dial(URL)
	if err != nil {
		fmt.Println("Dial ", err.Error())
	}

	from := "jordan.schalm@gmail.com"
	to := "somebody.else@gmail.com"

	if err = c.Mail(from); err != nil {
		fmt.Println("Mail ", err.Error())
	}

	if err = c.Rcpt(to); err != nil {
		fmt.Println("Rcpt ", err.Error())
	}

	wr, err := c.Data()
	if err != nil {
		fmt.Println("Data ", err.Error())
	}
	defer wr.Close()

	msg := fmt.Sprint("Subject: something\n")
	msg += "From: " + from + "\n"
	msg += "To: " + to + "\n"
	msg += "\n\n"
	msg += "hello\n"

	_, err = fmt.Fprint(wr, msg)
	if err != nil {
		fmt.Println("Send ", err.Error())
	}

	err = c.Quit()
	if err != nil {
		fmt.Println("Quit ", err.Error())
	}
}
