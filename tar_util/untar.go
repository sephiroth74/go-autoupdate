package tar_util

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
func Untar(dst string, r io.Reader) ([]string, error) {
	gzr, err := gzip.NewReader(r)
	var result []string
	if err != nil {
		return result, err
	}
	defer func(gzr *gzip.Reader) { _ = gzr.Close() }(gzr)

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return result, nil

		// return any other error
		case err != nil:
			return result, err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)
		if !strings.HasPrefix(header.Name, "._") {
			result = append(result, target)
		}

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return result, err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			baseDir := filepath.Dir(target)
			if _, err := os.Stat(baseDir); err != nil {
				if err := os.MkdirAll(baseDir, 0755); err != nil {
					return result, err
				}
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return result, err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return result, err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			_ = f.Close()
		}
	}
}
