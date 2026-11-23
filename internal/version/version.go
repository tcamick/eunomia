// Package version exposes the build version, set at link time by the Makefile
// (-ldflags "-X github.com/tcamick/eunomia/internal/version.Version=...").
package version

// Version is the build version; "dev" when built without the Makefile.
var Version = "dev"
