package main

import (
	"github.com/flashmob/go-guerrilla/dashboard"
)

func main() {
	dashboard.Run(&dashboard.Config{
		ListenInterface: ":8080",
	})
}
