package fileio

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// TestMain runs this test binary as another program that holds a lock,
// when a test asks for one, see holdLock.
func TestMain(m *testing.M) {
	if path := os.Getenv("FRAMEFAIRY_HOLD_LOCK"); path != "" {
		holdLock(path)
		return
	}
	os.Exit(m.Run())
}

// holdLock is this test binary run as another program that edits a file:
// it takes the file's lock, says so, and holds it until its input closes.
//
// On the real disk: it stands for another program, and the lock between
// two programs is the system's.
func holdLock(path string) {
	release := Lock(afero.NewOsFs(), path)
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
	release()
}

// waitsOn is a waits for lock, and a channel that has a value each time
// the lock it is handed to finds the file's lock held and waits for it.
func waitsOn() (func(string), <-chan struct{}) {
	c := make(chan struct{}, 1)
	return func(string) {
		select {
		case c <- struct{}{}:
		default:
		}
	}, c
}

// A plan in one memory does not wait for a plan at the same path in
// another, so tests that each hold their own run side by side. The same
// memory waits, as the disk does.
func TestALockIsTheLockOfOneFileSystem(t *testing.T) {
	const path = "/ep.framefairy/logs/clips.json"
	a, b := afero.NewMemMapFs(), afero.NewMemMapFs()
	unlock := Lock(a, path)
	// Both edits below say here when they wait.
	told, waits := waitsOn()
	done := make(chan struct{})
	go func() {
		lock(b, path, told)()
		close(done)
	}()
	select {
	case <-done:
	case <-waits:
		unlock()
		t.Fatal("a plan in another memory waited for this one")
	}
	same := make(chan struct{})
	go func() {
		lock(a, path, told)()
		close(same)
	}()
	// An edit that did not wait never says so, and hangs here until go
	// test's -timeout.
	<-waits
	select {
	case <-same:
		t.Fatal("a second edit of the same plan did not wait")
	default:
	}
	unlock()
	<-same
}

// An edit waits for another program editing the same file, the command
// line or a second copy of the app, and goes ahead once it is done.
//
// On the real disk, because the lock between two programs is the system's.
func TestALockWaitsForAnotherProgram(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no lock across programs on Windows yet")
	}
	path := filepath.Join(t.TempDir(), "clips.json")
	other := exec.Command(os.Args[0])
	other.Env = append(os.Environ(), "FRAMEFAIRY_HOLD_LOCK="+path)
	stdin, err := other.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := other.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close(); _ = other.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "held" {
		t.Fatalf("the other program said %q, %v", line, err)
	}

	told, waits := waitsOn()
	got := make(chan struct{})
	go func() {
		release := lock(afero.NewOsFs(), path, told)
		release()
		close(got)
	}()
	// An edit that does not wait for the other program never says so, and
	// hangs here until go test's -timeout.
	<-waits
	select {
	case <-got:
		t.Fatal("the lock was taken while another program held it")
	default:
	}
	stdin.Close()
	// One that is never let in hangs here.
	<-got
}
