package staticfiles

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const MaxUploadBytes int64 = 50 << 20
const MaxExpandedBytes int64 = 100 << 20
const MaxFiles = 5000

type Summary struct {
	Files int
	Bytes int64
}

// Extract writes only ordinary public files into a new, private staging directory.
// No uploaded path can address the instance's database, hooks, or admin/API routes.
func Extract(source *os.File, format, destination string) (Summary, error) {
	var result Summary
	info, err := source.Stat()
	if err != nil {
		return result, err
	}
	if info.Size() == 0 || info.Size() > MaxUploadBytes {
		return result, errors.New("upload must be between 1 byte and 50 MiB")
	}
	if format == "html" {
		_, err = source.Seek(0, io.SeekStart)
		if err == nil {
			err = writeFile(filepath.Join(destination, "index.html"), source, info.Size())
		}
		return Summary{Files: 1, Bytes: info.Size()}, err
	}
	if format != "zip" {
		return result, errors.New("upload a ZIP archive or an HTML file")
	}
	archive, err := zip.NewReader(source, info.Size())
	if err != nil {
		return result, errors.New("the file is not a valid ZIP archive")
	}
	if len(archive.File) > MaxFiles {
		return result, errors.New("ZIP archive contains too many entries (maximum 5000)")
	}
	type entry struct {
		file *zip.File
		name string
	}
	entries := []entry{}
	rootIndex := false
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "__MACOSX/") || path.Base(file.Name) == ".DS_Store" {
			continue
		}
		name := strings.TrimPrefix(file.Name, "./")
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		if strings.ContainsAny(name, "\\:\x00") || path.IsAbs(name) {
			return result, fmt.Errorf("invalid ZIP path: %s", file.Name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || part == "." || part == ".." || (strings.HasPrefix(part, ".") && part != ".well-known") {
				return result, fmt.Errorf("hidden files and unsafe paths cannot be published: %s", file.Name)
			}
			for _, character := range part {
				if character < 32 || character == 127 {
					return result, errors.New("ZIP paths cannot contain control characters")
				}
			}
		}
		if file.Mode().IsDir() {
			continue
		}
		if !file.Mode().IsRegular() {
			return result, errors.New("ZIP archives cannot contain symlinks or special files")
		}
		if file.UncompressedSize64 > uint64(MaxUploadBytes) {
			return result, errors.New("an individual file exceeds 50 MiB")
		}
		result.Bytes += int64(file.UncompressedSize64)
		if result.Bytes > MaxExpandedBytes {
			return result, errors.New("expanded site exceeds 100 MiB")
		}
		rootIndex = rootIndex || name == "index.html"
		entries = append(entries, entry{file, name})
	}
	// Accept either the build contents or a single enclosing directory (e.g. dist/).
	prefix := ""
	if !rootIndex && len(entries) > 0 {
		first, _, ok := strings.Cut(entries[0].name, "/")
		if ok {
			prefix = first + "/"
			for _, item := range entries {
				if !strings.HasPrefix(item.name, prefix) {
					prefix = ""
					break
				}
			}
		}
	}
	seen := map[string]bool{}
	hasIndex := false
	for _, item := range entries {
		name := strings.TrimPrefix(item.name, prefix)
		first := strings.ToLower(strings.Split(name, "/")[0])
		if first == "_" || first == "api" || first == "pb_data" || first == "pb_hooks" || first == "pb_migrations" {
			return result, fmt.Errorf("%s is reserved for PocketBase", first)
		}
		if seen[name] {
			return result, fmt.Errorf("duplicate ZIP path: %s", name)
		}
		seen[name] = true
		hasIndex = hasIndex || name == "index.html"
	}
	if !hasIndex {
		return result, errors.New("the site must contain index.html at its root (a single enclosing build folder is also accepted)")
	}
	for _, item := range entries {
		name := strings.TrimPrefix(item.name, prefix)
		destinationPath := filepath.Join(destination, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(destinationPath), 0700); err != nil {
			return result, err
		}
		reader, openErr := item.file.Open()
		if openErr != nil {
			return result, openErr
		}
		err = writeFile(destinationPath, reader, int64(item.file.UncompressedSize64))
		closeErr := reader.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	result.Files = len(entries)
	return result, nil
}

func writeFile(destination string, source io.Reader, expected int64) error {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(file, io.LimitReader(source, expected+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if n != expected {
		return errors.New("file length does not match the ZIP header")
	}
	return closeErr
}
