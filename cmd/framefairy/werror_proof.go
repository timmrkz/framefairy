package main

// A temporary proof for #688: with WERROR=1 this warning must fail the
// Mac build. The commit after this one removes it again.

// #warning "a warning that WERROR=1 turns into an error"
import "C"
