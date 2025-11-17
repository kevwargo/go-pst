package main

import (
	"log"

	"github.com/kevwargo/go-pst/cmd/pst"
)

func main() {
	if err := pst.Execute(); err != nil {
		log.Fatal(err)
	}
}
