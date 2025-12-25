package main

import (
	"log"

	"github.com/kevwargo/go-pst/cmd/pst"
)

func main() {
	log.SetFlags(log.Flags() | log.Lmicroseconds)

	if err := pst.Execute(); err != nil {
		log.Fatal(err)
	}
}
