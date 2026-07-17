package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNumPagesFor(t *testing.T) {
	cases := []struct {
		count, perPage, want int
	}{
		{0, 12, 0},
		{1, 12, 1},
		{12, 12, 1},
		{13, 12, 2},
		{24, 12, 2},
		{25, 12, 3},
		{15, 0, 15}, // perPage<=0 falls back to 1
	}
	for _, c := range cases {
		if got := numPagesFor(c.count, c.perPage); got != c.want {
			t.Errorf("numPagesFor(%d, %d) = %d, want %d", c.count, c.perPage, got, c.want)
		}
	}
}

func TestCleanDir(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.png", "sub/dir/c.jpg"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, n)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := cleanDir(dir); err != nil {
		t.Fatalf("cleanDir: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dir not empty after cleanDir: %d entries", len(entries))
	}

	// Missing dir is treated as already clean (no error).
	if err := cleanDir(filepath.Join(dir, "does-not-exist")); err != nil {
		t.Errorf("cleanDir on missing dir returned error: %v", err)
	}
}

func TestOutputExt(t *testing.T) {
	cases := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/gif":  ".jpg",
		"image/webp": ".jpg",
	}
	for ct, want := range cases {
		if got := outputExt(ct); got != want {
			t.Errorf("outputExt(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestStableName(t *testing.T) {
	a := stableName("photo.jpg")
	b := stableName("photo.jpg")
	if a != b {
		t.Errorf("stableName not deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("stableName length = %d, want 16", len(a))
	}
	if stableName("photo.jpg") == stableName("photo.png") {
		t.Error("stableName collided for different inputs")
	}
}

func TestValidateConfig(t *testing.T) {
	valid := Config{ImagesPerPage: 12, FullSizeWidth: 1500, ThumbnailSize: 400, JPEGQuality: 95, InputDir: "./input"}
	if err := validateConfig(&valid); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}

	bad := []Config{
		{ImagesPerPage: 0, FullSizeWidth: 1500, ThumbnailSize: 400, JPEGQuality: 95, InputDir: "./input"},
		{ImagesPerPage: 12, FullSizeWidth: 0, ThumbnailSize: 400, JPEGQuality: 95, InputDir: "./input"},
		{ImagesPerPage: 12, FullSizeWidth: 1500, ThumbnailSize: 0, JPEGQuality: 95, InputDir: "./input"},
		{ImagesPerPage: 12, FullSizeWidth: 1500, ThumbnailSize: 400, JPEGQuality: 0, InputDir: "./input"},
		{ImagesPerPage: 12, FullSizeWidth: 1500, ThumbnailSize: 400, JPEGQuality: 101, InputDir: "./input"},
		{ImagesPerPage: 12, FullSizeWidth: 1500, ThumbnailSize: 400, JPEGQuality: 95, InputDir: ""},
	}
	for i, c := range bad {
		if err := validateConfig(&c); err == nil {
			t.Errorf("case %d: invalid config accepted", i)
		}
	}
}
