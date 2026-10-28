package staticfiles

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

type testEntry struct {
	name, data string
	mode       os.FileMode
}

func zipFile(t *testing.T, entries []testEntry) *os.File {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "site-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(buffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func TestBuildFolderAndAssets(t *testing.T) {
	destination := t.TempDir()
	archive := zipFile(t, []testEntry{{name: "dist/index.html", data: "<h1>Site</h1>"}, {name: "dist/assets/app.js", data: "console.log('ok')"}, {name: "__MACOSX/._index.html", data: "ignored"}})
	result, err := Extract(archive, "zip", destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 || result.Bytes != 30 {
		t.Fatalf("wrong summary: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(destination, "assets", "app.js"))
	if err != nil || string(data) != "console.log('ok')" {
		t.Fatalf("missing asset: %s %v", data, err)
	}
	if _, err = os.Stat(filepath.Join(destination, "index.html")); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeOrInvalidArchives(t *testing.T) {
	for _, entry := range []testEntry{
		{name: "../escaped.html", data: "bad"}, {name: "/absolute.html", data: "bad"},
		{name: "nested/../../escaped.html", data: "bad"}, {name: "assets\\bad.js", data: "bad"},
		{name: "C:/bad.html", data: "bad"}, {name: ".env", data: "secret"},
		{name: "api/health", data: "override"}, {name: "_/index.html", data: "override"},
		{name: "pb_hooks/main.pb.js", data: "execute"}, {name: "pb_data/data.db", data: "overwrite"},
		{name: "link", data: "../../pb_data", mode: os.ModeSymlink | 0777},
		{name: "index.html", data: "duplicate"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			destination := t.TempDir()
			archive := zipFile(t, []testEntry{{name: "index.html", data: "good"}, entry})
			if _, err := Extract(archive, "zip", destination); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
		})
	}
	missing := zipFile(t, []testEntry{{name: "assets/app.js", data: "ok"}})
	if _, err := Extract(missing, "zip", t.TempDir()); err == nil {
		t.Fatal("site without index was accepted")
	}
}

func TestSingleHTMLAndExpandedLimit(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "page-*.html")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_, _ = file.WriteString("<h1>Standalone</h1>")
	destination := t.TempDir()
	if result, err := Extract(file, "html", destination); err != nil || result.Files != 1 {
		t.Fatalf("HTML failed: %+v %v", result, err)
	}
	archive := zipFile(t, []testEntry{{name: "index.html", data: string(bytes.Repeat([]byte("x"), int(MaxUploadBytes+1)))}})
	if _, err := Extract(archive, "zip", t.TempDir()); err == nil {
		t.Fatal("expanded file limit was not enforced")
	}
}
