package main

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
	helos = []string{"hi", "hello", "ahoy", "bonjour"}
	froms = []string{"joe@gmail.com", "jane@gmail.com", "alex@gmail.com", "sally@gmail.com"}
)

func lastWords(message string, err error) {
	fmt.Println(message, err.Error())
	return
	// panic(err)
}

// Sends a single SMTP message, for testing.
func main() {
	for {
		time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 10)))
		go sendMail(
			helos[rand.Int()%4],
			froms[rand.Int()%4],
		)
	}
}

func sendMail(helo, from string) {
	// fmt.Printf("Sending mail")
	c, err := smtp.Dial(URL)
	if err != nil {
		lastWords("Dial ", err)
	}
	defer c.Close()

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

	if err = c.Hello(helo); err != nil {
		lastWords("Hello ", err)
	}

	if err = c.Mail(from); err != nil {
		lastWords("Mail ", err)
	}

	to := "recipient@gmail.com"
	if err = c.Rcpt(to); err != nil {
		lastWords("Rcpt ", err)
	}

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

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

	// Introduce some artificial delay
	time.Sleep(time.Millisecond * (time.Duration(rand.Int() % 50)))

	err = c.Quit()
	if err != nil {
		lastWords("Quit ", err)
	}
}
