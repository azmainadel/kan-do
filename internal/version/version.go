// Package version is the kan-do version, set at build time:
//
//	go build -ldflags "-X github.com/azmainadel/kan-do/internal/version.Version=1.2.3"
package version

// Version is this build's release, without a leading v.
var Version = "0.1.0-dev"
