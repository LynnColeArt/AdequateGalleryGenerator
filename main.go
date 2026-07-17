package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

type Config struct {
	ImagesPerPage int    `json:"imagesPerPage"`
	FullSizeWidth int    `json:"fullSizeWidth"`
	ThumbnailSize int    `json:"thumbnailSize"`
	JPEGQuality   int    `json:"jpegQuality"`
	InputDir      string `json:"inputDir"`
	OutputDir     string `json:"outputDir"`
	TemplatePath  string `json:"templatePath"`
}

type Image struct {
	Name        string
	DisplayPath string
	ThumbPath   string
}

type PageData struct {
	Images      []Image
	CurrentPage int
	TotalPages  int
	PageNumbers []int
}

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	configFile, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file %q: %v", *configPath, err)
	}

	var config Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	applyDefaults(&config)
	if err := validateConfig(&config); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	fullSizeDir := filepath.Join(config.OutputDir, "full")
	thumbDir := filepath.Join(config.OutputDir, "thumb")

	images, err := processImages(config.InputDir, fullSizeDir, thumbDir, config.FullSizeWidth, config.ThumbnailSize, config.JPEGQuality)
	if err != nil {
		log.Fatalf("Failed to process images: %v", err)
	}
	if len(images) == 0 {
		log.Fatalf("No images were generated from %q (check that it contains image files)", config.InputDir)
	}

	if err := generateGallery(images, config.TemplatePath, config.OutputDir, config.ImagesPerPage); err != nil {
		log.Fatalf("Failed to generate gallery: %v", err)
	}

	log.Printf("Generated %d image(s) into %s", len(images), config.OutputDir)
}

func applyDefaults(c *Config) {
	if c.OutputDir == "" {
		c.OutputDir = "./output"
	}
	if c.TemplatePath == "" {
		c.TemplatePath = "gallery.gohtml"
	}
	if c.JPEGQuality == 0 {
		c.JPEGQuality = 95
	}
}

func validateConfig(c *Config) error {
	if c.InputDir == "" {
		return fmt.Errorf("inputDir must be set")
	}
	if c.ImagesPerPage <= 0 {
		return fmt.Errorf("imagesPerPage must be greater than 0")
	}
	if c.FullSizeWidth <= 0 {
		return fmt.Errorf("fullSizeWidth must be greater than 0")
	}
	if c.ThumbnailSize <= 0 {
		return fmt.Errorf("thumbnailSize must be greater than 0")
	}
	if c.JPEGQuality < 1 || c.JPEGQuality > 100 {
		return fmt.Errorf("jpegQuality must be between 1 and 100")
	}
	return nil
}

// cleanDir removes every entry inside dir (but not dir itself). A missing dir
// is treated as already clean.
func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// outputExt returns the web-safe extension to use for a processed image based on
// its sniffed content type. PNGs are kept lossless so transparency survives;
// everything else is flattened to JPEG.
func outputExt(contentType string) string {
	if contentType == "image/png" {
		return ".png"
	}
	return ".jpg"
}

// stableName derives a deterministic, collision-resistant filename from the
// original name. Re-running the generator no longer shuffles filenames, so
// external links to full/<name> survive regeneration.
func stableName(originalName string) string {
	sum := sha1.Sum([]byte(originalName))
	return hex.EncodeToString(sum[:])[:16]
}

func processImages(inputDir string, fullSizeDir string, thumbDir string, fullSizeWidth int, thumbnailSize int, jpegQuality int) ([]Image, error) {
	// Ensure output directories exist (the old version silently failed if they
	// didn't, logging every save error and exiting 0 with an empty gallery).
	if err := os.MkdirAll(fullSizeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create full size dir: %w", err)
	}
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return nil, fmt.Errorf("create thumbnail dir: %w", err)
	}

	// Start each run from a clean slate so re-runs don't accumulate orphan
	// files alongside the freshly generated ones.
	if err := cleanDir(fullSizeDir); err != nil {
		return nil, fmt.Errorf("clean full size dir: %w", err)
	}
	if err := cleanDir(thumbDir); err != nil {
		return nil, fmt.Errorf("clean thumbnail dir: %w", err)
	}

	inputFiles, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	// Preserve input (sorted) order in the output by writing each result back
	// to its original index. A small worker pool processes images concurrently
	// while keeping the rest of the tool dog simple.
	images := make([]Image, len(inputFiles))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup

	for i, inputFile := range inputFiles {
		if inputFile.IsDir() {
			continue
		}
		i, inputFile := i, inputFile

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			img, ok := processOne(inputDir, inputFile.Name(), fullSizeDir, thumbDir, fullSizeWidth, thumbnailSize, jpegQuality)
			if ok {
				images[i] = img
			}
		}()
	}
	wg.Wait()

	// Drop skipped entries (non-images, decode failures) in order.
	result := images[:0]
	for _, im := range images {
		if im.Name != "" {
			result = append(result, im)
		}
	}
	return result, nil
}

// processOne handles a single input file: sniffs, decodes (with EXIF
// auto-orientation), resizes, and saves full + thumbnail. Returns ok=false for
// any file that should be skipped (the reason is already logged).
func processOne(inputDir, name, fullSizeDir, thumbDir string, fullSizeWidth, thumbnailSize, jpegQuality int) (Image, bool) {
	inputPath := filepath.Join(inputDir, name)

	// Sniff content type, then close the handle immediately. The old code
	// deferred the close inside the loop, leaking a file descriptor per image.
	sniffFile, err := os.Open(inputPath)
	if err != nil {
		log.Printf("Failed to open file %s: %v", inputPath, err)
		return Image{}, false
	}
	buffer := make([]byte, 512)
	n, err := sniffFile.Read(buffer)
	sniffFile.Close()
	if err != nil && err != io.EOF {
		log.Printf("Failed to read file %s: %v", inputPath, err)
		return Image{}, false
	}

	contentType := http.DetectContentType(buffer[:n])
	if !strings.HasPrefix(contentType, "image/") {
		log.Printf("File %s is not an image: %s", inputPath, contentType)
		return Image{}, false
	}

	// AutoOrientation makes phone photos (rotation stored as EXIF, not pixels)
	// appear upright instead of sideways.
	img, err := imaging.Open(inputPath, imaging.AutoOrientation(true))
	if err != nil {
		log.Printf("Failed to open image %s: %v", inputPath, err)
		return Image{}, false
	}

	ext := outputExt(contentType)
	filename := stableName(name) + ext

	// Web paths must use forward slashes regardless of OS; filepath.Join would
	// emit backslashes on Windows and break the href/src in the generated HTML.
	webPath := func(subdir string) string { return path.Join(subdir, filename) }

	fullSizeImg := imaging.Fit(img, fullSizeWidth, fullSizeWidth, imaging.Lanczos)
	fullSizeOutputPath := filepath.Join(fullSizeDir, filename)
	if err := saveImage(fullSizeImg, fullSizeOutputPath, ext, jpegQuality); err != nil {
		log.Printf("Failed to save full size image %s: %v", fullSizeOutputPath, err)
		return Image{}, false
	}

	thumbImg := imaging.Thumbnail(img, thumbnailSize, thumbnailSize, imaging.Lanczos)
	thumbOutputPath := filepath.Join(thumbDir, filename)
	if err := saveImage(thumbImg, thumbOutputPath, ext, jpegQuality); err != nil {
		log.Printf("Failed to save thumbnail image %s: %v", thumbOutputPath, err)
		return Image{}, false
	}

	return Image{
		Name:        name,
		DisplayPath: webPath("full"),
		ThumbPath:   webPath("thumb"),
	}, true
}

// saveImage writes an image, applying JPEG quality for JPEG output. PNG output
// is lossless and ignores the quality setting.
func saveImage(img image.Image, dest, ext string, jpegQuality int) error {
	opts := []imaging.EncodeOption{}
	if ext == ".jpg" {
		opts = append(opts, imaging.JPEGQuality(jpegQuality))
	}
	return imaging.Save(img, dest, opts...)
}

// numPagesFor returns the number of pagination pages needed for count images at
// perPage images per page.
func numPagesFor(count, perPage int) int {
	if perPage <= 0 {
		perPage = 1
	}
	return (count + perPage - 1) / perPage
}

func generateGallery(images []Image, tmplPath string, outputDir string, imagesPerPage int) error {
	funcMap := template.FuncMap{
		"minus": func(a, b int) int { return a - b },
		"plus":  func(a, b int) int { return a + b },
	}

	tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(funcMap).ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	// Wipe stale gallery pages and index from any previous run so we don't
	// leave dead links behind when the image count shrinks.
	oldPages, err := filepath.Glob(filepath.Join(outputDir, "gallery*.html"))
	if err != nil {
		return fmt.Errorf("glob old pages: %w", err)
	}
	for _, p := range oldPages {
		if err := os.Remove(p); err != nil {
			return fmt.Errorf("remove old page %s: %w", p, err)
		}
	}
	indexPath := filepath.Join(outputDir, "index.html")
	os.Remove(indexPath) // best-effort; recreated below if there are pages

	numPages := numPagesFor(len(images), imagesPerPage)
	pageNumbers := make([]int, numPages)
	for i := range pageNumbers {
		pageNumbers[i] = i + 1
	}

	renderPage := func(pageNum int, data PageData) error {
		outputPath := filepath.Join(outputDir, "gallery"+strconv.Itoa(pageNum)+".html")
		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		// Execute then close immediately so file handles never accumulate
		// across pages (the old code deferred this inside the loop).
		execErr := tmpl.Execute(f, data)
		if err := f.Close(); err != nil {
			return err
		}
		return execErr
	}

	for i := 0; i < numPages; i++ {
		start := i * imagesPerPage
		end := start + imagesPerPage
		if end > len(images) {
			end = len(images)
		}

		data := PageData{
			Images:      images[start:end],
			CurrentPage: i + 1,
			TotalPages:  numPages,
			PageNumbers: pageNumbers,
		}

		if err := renderPage(i+1, data); err != nil {
			return err
		}
	}

	// Mirror the first page as index.html so visitors don't hit a directory
	// listing when they land on the output folder.
	if numPages > 0 {
		if err := copyFile(filepath.Join(outputDir, "gallery1.html"), indexPath); err != nil {
			return fmt.Errorf("write index.html: %w", err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
