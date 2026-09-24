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
	lf, err := openLogFile()
	if err != nil {
		log.Println(err)

		return func() {}
	}

	wBak := log.Writer()
	log.SetOutput(lf)
	log.SetPrefix(fmt.Sprintf("[PID %d] ", os.Getpid()))
	log.Printf("Started.")

	return func() {
		log.Printf("Closing...")
		lf.Close()
		log.SetOutput(wBak)
	}
}

func openLogFile() (*os.File, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolving user cache dir: %w", err)
	}

	lf, err := os.OpenFile(filepath.Join(cacheDir, "pst.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	return lf, nil
}
