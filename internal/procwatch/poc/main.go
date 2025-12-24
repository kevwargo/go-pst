package main

import (
	"log"
	"os"

	"github.com/kevwargo/go-pst/internal/procwatch"
)

func main() {
	log.SetFlags(log.Flags() | log.Lmicroseconds)

	w, err := procwatch.Watch()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			msg, err := w.Recv()
			if err != nil || msg == nil {
				log.Printf("Last Recv() = (%v, %v)", msg, err)
				return
			}
			log.Printf("Recv() = (%v, %v)", msg, err)
		}
	}()

	var reply [1]byte
	log.Println("waiting for Enter to close socket")
	os.Stdin.Read(reply[:])

	log.Println("closing socket")
	w.Close()

	log.Println("waiting for Enter to finish process")
	os.Stdin.Read(reply[:])
}
