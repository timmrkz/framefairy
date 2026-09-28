//go:build !darwin

package asr

// fastCores is zero where the system does not tell performance cores from
// others, and every core counts.
func fastCores() int { return 0 }
