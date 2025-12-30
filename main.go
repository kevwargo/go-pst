package main

import (
	"log"

	"github.com/kevwargo/go-pst/cmd"
)

func main() {
	log.SetFlags(log.Flags() | log.Lmicroseconds)

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
