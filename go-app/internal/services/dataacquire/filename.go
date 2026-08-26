package dataacquire

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// ArchiveExt is the only archive form this package accepts.
const ArchiveExt = ".zip"

// ErrUnsafeFilename is returned when a URL's last path segment cannot be used as a
// filename without leaving the staging directory.
var ErrUnsafeFilename = errors.New("url does not name a safe archive file")

// ArchiveFilename derives the staging filename from a URL.
//
// The last path segment of a URL is attacker-influenced even behind an allowlist —
// a path of /downloads/../../etc/passwd is a perfectly valid URL. Deriving the name
// here, once, and rejecting anything that is not a plain "<name>.zip" means no caller
// has to remember to sanitise it. This is the same class of defect as the zip-slip
// A-2 guards against, one layer earlier.
func ArchiveFilename(u *url.URL) (string, error) {
	name := path.Base(path.Clean("/" + u.EscapedPath()))
	unescaped, err := url.PathUnescape(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnsafeFilename, name)
	}
	name = unescaped

	if name == "" || name == "." || name == "/" || name == ".." {
		return "", fmt.Errorf("%w: url path ends in no filename", ErrUnsafeFilename)
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: %q", ErrUnsafeFilename, name)
	}
	if !strings.EqualFold(path.Ext(name), ArchiveExt) {
		return "", fmt.Errorf("%w: %q is not a %s archive", ErrUnsafeFilename, name, ArchiveExt)
	}
	return name, nil
}
