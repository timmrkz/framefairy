package stdlog

import "log"

// What writes to the standard library's log, which reaches no log a
// report sends.
func say(err error) {
	log.Printf("failed: %v", err)
	log.Println("failed")
	log.Fatal(err)
}
