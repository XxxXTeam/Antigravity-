package embed

import (
	"embed"
	"io/fs"
)

// PublicFS contains the embedded public files (admin UI)
//
//go:embed all:public
var publicFS embed.FS

// GetPublicFS returns the embedded public filesystem
// This allows serving the admin UI without external files
func GetPublicFS() (fs.FS, error) {
	return fs.Sub(publicFS, "public")
}

// HasEmbeddedFiles checks if public files are embedded
func HasEmbeddedFiles() bool {
	entries, err := publicFS.ReadDir("public")
	return err == nil && len(entries) > 0
}
