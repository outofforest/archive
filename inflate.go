package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/ulikunitz/xz"
)

// ErrUnknownArchiveFormat is error returned when file is not recognized as archive.
var ErrUnknownArchiveFormat = errors.New("unknown archive format")

// Inflate inflates stream based on file name.
func Inflate(name string, reader io.Reader, path string) error {
	switch {
	case strings.HasSuffix(name, ".tar"):
		return InflateTar(reader, path)
	case strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"):
		return InflateTarGz(reader, path)
	case strings.HasSuffix(name, ".tar.xz"):
		return InflateTarXz(reader, path)
	case strings.HasSuffix(name, ".zip"):
		return InflateZip(reader, path)
	default:
		return errors.Wrapf(ErrUnknownArchiveFormat, "unknown format of %s", name)
	}
}

// InflateTar inflates tar archive.
func InflateTar(reader io.Reader, path string) (retErr error) {
	tmpPath := path + ".tmp"
	tmpPath, err := ensureDir(tmpPath)
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(tmpPath)
		}
	}()

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			if err := drain(reader); err != nil {
				return err
			}
			if err := os.Rename(tmpPath, path); err != nil {
				return err
			}
			return nil
		case err != nil:
			return errors.WithStack(err)
		case header == nil || header.Name == "pax_global_header":
			continue
		}

		header.Name, err = ensureFileInDir(tmpPath, header.Name)
		if err != nil {
			return err
		}
		if header.Name == "" {
			continue
		}

		// We take mode from header.FileInfo().Mode(), not from header.Mode because they may be in
		// different formats (meaning of bits may be different).
		// header.FileInfo().Mode() returns compatible value.
		mode := header.FileInfo().Mode()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(header.Name, mode); err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeReg:
			if err := ensureFileDir(header.Name); err != nil {
				return err
			}

			f, err := os.OpenFile(header.Name, os.O_CREATE|os.O_WRONLY, mode)
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = io.Copy(f, tr)
			_ = f.Close()
			if err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeSymlink:
			if err := ensureFileDir(header.Name); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, header.Name); err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeLink:
			header.Linkname = path + "/" + header.Linkname
			if err := ensureFileDir(header.Name); err != nil {
				return err
			}
			if err := ensureFileDir(header.Linkname); err != nil {
				return err
			}
			// linked file may not exist yet, so let's create it - it will be overwritten later
			f, err := os.OpenFile(header.Linkname, os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				if !os.IsExist(err) {
					return errors.WithStack(err)
				}
			} else {
				_ = f.Close()
			}
			if err := os.Link(header.Linkname, header.Name); err != nil {
				return errors.WithStack(err)
			}
		default:
			return errors.Errorf("unsupported file type: %d for %s", header.Typeflag, header.Name)
		}
	}
}

// InflateTarGz inflates tar.gz archive.
func InflateTarGz(reader io.Reader, path string) error {
	reader2, err := gzip.NewReader(reader)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := InflateTar(reader2, path); err != nil {
		return err
	}
	return drain(reader)
}

// InflateTarXz inflates tar.xz archive.
func InflateTarXz(reader io.Reader, path string) error {
	reader2, err := xz.NewReader(reader)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := InflateTar(reader2, path); err != nil {
		return err
	}
	return drain(reader)
}

// InflateZip inflates zip archive.
func InflateZip(reader io.Reader, path string) (retErr error) {
	tmpPath := path + ".tmp"
	tmpPath, err := ensureDir(tmpPath)
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(tmpPath)
		}
	}()

	file, ok := reader.(*os.File)
	if !ok {
		// Create a temporary file
		tempFile, err := os.Create(tmpPath + ".zip")
		if err != nil {
			return errors.WithStack(err)
		}
		defer os.Remove(tempFile.Name()) //nolint: errcheck

		// Copy the contents of the reader to the temporary file
		_, err = io.Copy(tempFile, reader)
		if err != nil {
			return errors.WithStack(err)
		}

		// Open the temporary file for reading
		file2, err := os.Open(tempFile.Name())
		if err != nil {
			return errors.WithStack(err)
		}
		defer file2.Close()
		file = file2
	}

	// Get the file information to obtain its size
	fileInfo, err := file.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	fileSize := fileInfo.Size()

	// Use the file as a ReaderAt to unpack the zip file
	zipReader, err := zip.NewReader(file, fileSize)
	if err != nil {
		return errors.WithStack(err)
	}

	// Process the files in the zip archive
	for _, zf := range zipReader.File {
		// Open each file in the archive
		rc, err := zf.Open()
		if err != nil {
			return errors.WithStack(err)
		}
		defer rc.Close()

		// Construct the destination path for the file
		dst, err := ensureFileInDir(tmpPath, zf.Name)
		if err != nil {
			return err
		}
		if dst == "" {
			continue
		}

		mode := zf.FileInfo().Mode()

		//nolint:nestif
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, mode); err != nil {
				return errors.WithStack(err)
			}
		} else {
			if err := ensureFileDir(dst); err != nil {
				return err
			}

			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, mode)
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = io.Copy(f, rc)
			_ = f.Close()
			_ = rc.Close()
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return errors.WithStack(os.Rename(tmpPath, path))
}

// drain is used to read all remaining bytes from the reader.
// We found that tar.xz library archive doesn't necessarily read all the bytes.
// We fix it here so the hasher computes correct hash.
func drain(reader io.Reader) error {
	_, err := io.ReadAll(reader)
	return errors.WithStack(err)
}
