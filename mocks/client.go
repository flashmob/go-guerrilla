package mocks

import (
	"fmt"
	"net/smtp"
	"time"
)

const (
	URL = "127.0.0.1:2500"
)

func lastWords(message string, err error) {
	fmt.Println(message, err.Error())
	return
	// panic(err)
}

// Sends a single SMTP message, for testing.
func main() {
	for i := 0; i < 100; i++ {
		go sendMail(i)
	}
	time.Sleep(time.Minute / 10)
}

func sendMail(i int) {
	fmt.Printf("Sending %d mail\n", i)
	c, err := smtp.Dial(URL)
	if err != nil {
		lastWords("Dial ", err)
	}
	defer c.Close()

	from := "somebody@gmail.com"
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

	fmt.Printf("About to quit %d\n", i)
	err = c.Quit()
	if err != nil {
		lastWords("Quit ", err)
	}
	fmt.Printf("Finished sending %d mail\n", i)
}
