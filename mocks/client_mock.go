package mocks

import (
	"fmt"
	"math/rand"
	"net/smtp"
	"time"
)

func lastWords(message string, err error) {
	fmt.Println(message, err.Error())
	return
	// panic(err)
}

type Client struct {
	Helo         string
	EmailAddress string
}

func (c *Client) SendMail(to, url string) {
	// fmt.Printf("Sending mail")
	sc, err := smtp.Dial(url)
	if err != nil {
		lastWords("Dial ", err)
	}
	defer sc.Close()

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

	if err = sc.Hello(c.Helo); err != nil {
		lastWords("Hello ", err)
	}

	if err = sc.Mail(c.EmailAddress); err != nil {
		lastWords("Mail ", err)
	}

	if err = sc.Rcpt(to); err != nil {
		lastWords("Rcpt ", err)
	}

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

	wr, err := sc.Data()
	if err != nil {
		lastWords("Data ", err)
	}
	defer wr.Close()

	msg := fmt.Sprint("Subject: something\n")
	msg += "From: " + c.EmailAddress + "\n"
	msg += "To: " + to + "\n"
	msg += "\n\n"
	msg += "hello\n"

	_, err = fmt.Fprint(wr, msg)
	if err != nil {
		lastWords("Send ", err)
	}

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

	err = sc.Quit()
	if err != nil {
		lastWords("Quit ", err)
	}
}
