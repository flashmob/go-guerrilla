package main

import (
	"github.com/jordanschalm/guerrilla/dashboard"
)

func main() {
	dashboard.Run(&dashboard.Config{
		Password:        "password",
		Username:        "admin",
		ListenInterface: ":8080",
	})
}
