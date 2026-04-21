package sandbox

import _ "embed"

// DefaultSeccompProfile is Docker's default seccomp profile, embedded at build time.
// It restricts dangerous syscalls while allowing common development operations.
// Source: github.com/moby/profiles/seccomp/default.json
//
//go:embed seccomp_default.json
var DefaultSeccompProfile string
