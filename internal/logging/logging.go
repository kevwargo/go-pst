package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func Init() {
	log.SetFlags(log.Flags() | log.Lmicroseconds)
}

func Redirect() func() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Println(err.Error())
		return func() {}
	}

	lf, err := os.OpenFile(filepath.Join(cacheDir, "pst.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		log.Printf("opening log file: %s", err)
		return func() {}
	}

	w := log.Writer()
	log.SetOutput(lf)
	log.SetPrefix(fmt.Sprintf("[PID %d] ", os.Getpid()))

	log.Printf("Started.")

	return func() {
		log.Printf("Closing...")
		lf.Close()
		log.SetOutput(w)
	}
}
