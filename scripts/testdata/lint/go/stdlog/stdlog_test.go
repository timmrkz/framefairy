package stdlog

import (
	"log"
	"testing"
)

// A test may write to the standard log, to prove where its lines go.
func TestSay(t *testing.T) {
	log.Printf("said")
}
