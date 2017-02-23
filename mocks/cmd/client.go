package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/flashmob/go-guerrilla/mocks"
)

const (
	URL = "127.0.0.1:2500"
)

var (
	helos  = []string{"hi", "hello", "ahoy", "bonjour", "hey!", "whats up"}
	emails = []string{
		"joe@yahoo.com",
		"jane@gmail.com",
		"alex@hotmail.com",
		"sally@fastmail.com",
		"alex@outlook.com",
		"barry@mail.com",
		"jill@email.net",
		"bob@greatmail.com",
		"jason@gmail.com",
		"tom@yahoo.ca",
	}
)

func main() {
	c := make(chan int)
	for i := 0; i < 100; i++ {
		go sendMailForever(time.Millisecond * time.Duration(rand.Int()%500))
	}
	<-c
}

func sendMailForever(wait time.Duration) {
	c := mocks.Client{
		Helo:         helos[rand.Int()%len(helos)],
		EmailAddress: emails[rand.Int()%len(emails)],
	}
	fmt.Println(c)

	for {
		c.SendMail("someone@gmail.com", URL)
		time.Sleep(wait)
	}
}
