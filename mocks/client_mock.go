package mocks

import (
	"fmt"
	"math/rand"
	"net/smtp"
	"time"
)

const (
	URL = "127.0.0.1:2500"
)

var (
	helos = []string{"hi", "hello", "ahoy", "bonjour", "hey!"}
	froms = []string{"joe@yahoo.com", "jane@gmail.com", "alex@hotmail.com", "sally@fastmail.com", "alex@outlook.com"}
)

func lastWords(message string, err error) {
	fmt.Println(message, err.Error())
	return
	// panic(err)
}

type Client struct {
	helo         string
	emailAddress string
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

	if err = sc.Hello(c.helo); err != nil {
		lastWords("Hello ", err)
	}

	if err = sc.Mail(c.emailAddress); err != nil {
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
	msg += "From: " + c.emailAddress + "\n"
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
