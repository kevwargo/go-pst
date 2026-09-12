package main

import (
	"log"

	"github.com/kevwargo/go-pst/cmd"
	"github.com/kevwargo/go-pst/internal/logging"
)

func main() {
	logging.Init()

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
