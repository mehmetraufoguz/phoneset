package camera

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The Windows camera comes as a zip (windows/release.ps1): the DirectShow DLL, the helper that carries the picture,
// and the licence of the DLL. The code that unpacks it is here, with no build tag, so that it is tested on every
// system, although only Windows uses it.

// Limits for what a camera zip may hold: a release's zip is checked against SHA256SUMS before it is opened, but a
// file that claims to be gigabytes, or a path that leaves the folder, would still be a hole.
const (
	maxZipFiles    = 32
	maxZipFileSize = 200 << 20 // the DLL and the helper are a few MB each
)

// extractZip unpacks the zip at src into dest. Every name must stay inside dest (an absolute path, a drive, or a
// ".." that leaves it is refused), only regular files and folders are taken (no links), and the number and size
// of the files are limited.
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("couldn't open the camera zip: %w", err)
	}
	defer r.Close()
	if len(r.File) > maxZipFiles {
		return fmt.Errorf("the camera zip has %d files, more than the %d expected", len(r.File), maxZipFiles)
	}
	root := filepath.Clean(dest)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		// A zip writes its paths with slashes; a backslash is a Windows path trying to be one (a drive, a "..\").
		if strings.ContainsRune(f.Name, '\\') {
			return fmt.Errorf("the camera zip has a backslash in a name: %q", f.Name)
		}
		name := filepath.FromSlash(f.Name)
		if filepath.IsAbs(name) || filepath.VolumeName(name) != "" || strings.HasPrefix(f.Name, "/") {
			return fmt.Errorf("the camera zip has an absolute path: %q", f.Name)
		}
		target := filepath.Join(root, name)
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("the camera zip has a path outside its folder: %q", f.Name)
		}
		mode := f.Mode()
		switch {
		case f.FileInfo().IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode.IsRegular():
			if err := writeZipFile(f, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the camera zip has %q, which is not a plain file", f.Name)
		}
	}
	return nil
}

func writeZipFile(f *zip.File, target string) error {
	if f.UncompressedSize64 > maxZipFileSize {
		return fmt.Errorf("%q in the camera zip claims %d bytes", f.Name, f.UncompressedSize64)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	// The size in the header could lie: read one byte more than allowed and refuse if it is there.
	n, err := io.Copy(out, io.LimitReader(rc, maxZipFileSize+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxZipFileSize {
		return fmt.Errorf("%q in the camera zip is bigger than %d bytes", f.Name, int64(maxZipFileSize))
	}
	return nil
}
