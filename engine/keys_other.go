//go:build !darwin

package engine

// Machines other than the Mac have no keychain the app uses yet, so a key
// comes from the environment only. Windows has the Credential Manager and
// Linux the Secret Service, and they are where keys go when those builds
// ship.
func systemKeys() keyStore { return noKeys{} }

func legacyKeys() keyStore { return noKeys{} }
