// Package framefairy holds the two files that say which version of other
// people's code the programs are built from: go.mod for the Go modules,
// and the interface's package-lock.json for the packages it bundles. They
// are the files Dependabot moves, and the Licences page reads its versions
// from them, so no notice keeps a copy that could fall behind, see
// notices. Only this folder can embed go.mod, which is why it is here.
package framefairy

import _ "embed"

// GoMod is go.mod.
//
//go:embed go.mod
var GoMod []byte

// PackageLock is frontend/package-lock.json.
//
//go:embed frontend/package-lock.json
var PackageLock []byte
