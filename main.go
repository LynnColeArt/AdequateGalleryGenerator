package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "html/template"
    "io/ioutil"
    "log"
    "math/rand"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    "github.com/disintegration/imaging"
)

type Config struct {
    ImagesPerPage int    `json:"imagesPerPage"`
    FullSizeWidth int    `json:"fullSizeWidth"`
    ThumbnailSize int    `json:"thumbnailSize"`
    InputDir      string `json:"inputDir"`
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
}

func main() {
    flag.Parse()

    configFile, err := ioutil.ReadFile("config.json")
    if err != nil {
        log.Fatalf("Failed to read config file: %v", err)
    }

    var config Config
    err = json.Unmarshal(configFile, &config)
    if err != nil {
        log.Fatalf("Failed to parse config file: %v", err)
    }

    images, err := processImages(config.InputDir, "./output/full", "./output/thumb", config.FullSizeWidth, config.ThumbnailSize)
    if err != nil {
        log.Fatalf("Failed to process images: %v", err)
    }

    err = generateGallery(images, "gallery.gohtml", "./output", config.ImagesPerPage)
    if err != nil {
        log.Fatalf("Failed to generate gallery: %v", err)
    }
}

func processImages(inputDir string, fullSizeDir string, thumbDir string, fullSizeWidth int, thumbnailSize int) ([]Image, error) {
    inputFiles, err := ioutil.ReadDir(inputDir)
    if err != nil {
        return nil, err
    }

    var images []Image
    for _, inputFile := range inputFiles {
        inputPath := filepath.Join(inputDir, inputFile.Name())

        file, err := os.Open(inputPath)
        if err != nil {
            log.Printf("Failed to open file %s: %v", inputPath, err)
            continue
        }
        defer file.Close()

        buffer := make([]byte, 512)
        _, err = file.Read(buffer)
        if err != nil {
            log.Printf("Failed to read file %s: %v", inputPath, err)
            continue
        }

        contentType := http.DetectContentType(buffer)
        if !strings.HasPrefix(contentType, "image/") {
            log.Printf("File %s is not an image: %s", inputPath, contentType)
            continue
        }

        img, err := imaging.Open(inputPath)
        if err != nil {
            log.Printf("Failed to open image %s: %v", inputPath, err)
            continue
        }

        rand.Seed(time.Now().UnixNano())
        filename := fmt.Sprintf("%d.jpg", rand.Int())

        fullSizeImg := imaging.Fit(img, fullSizeWidth, fullSizeWidth, imaging.Lanczos)
        fullSizeOutputPath := filepath.Join(fullSizeDir, filename)
        err = imaging.Save(fullSizeImg, fullSizeOutputPath)
        if err != nil {
            log.Printf("Failed to save full size image %s: %v", fullSizeOutputPath, err)
            continue
        }

        thumbImg := imaging.Thumbnail(img, thumbnailSize, thumbnailSize, imaging.Lanczos)
        thumbOutputPath := filepath.Join(thumbDir, filename)
        err = imaging.Save(thumbImg, thumbOutputPath)
        if err != nil {
            log.Printf("Failed to save thumbnail image %s: %v", thumbOutputPath, err)
            continue
        }

        images = append(images, Image{
            Name:        inputFile.Name(),
            DisplayPath: filepath.Join("full", filename),
            ThumbPath:   filepath.Join("thumb", filename),
        })
    }

    return images, nil
}

func generateGallery(images []Image, tmplPath string, outputDir string, imagesPerPage int) error {
    funcMap := template.FuncMap{
        "minus": func(a, b int) int {
            return a - b
        },
        "plus": func(a, b int) int {
            return a + b
        },
    }

    tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(funcMap).ParseFiles(tmplPath)
    if err != nil {
        return err
    }

    numPages := (len(images) + imagesPerPage - 1) / imagesPerPage
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
        }

        outputPath := filepath.Join(outputDir, "gallery"+strconv.Itoa(i+1)+".html")
        f, err := os.Create(outputPath)
        if err != nil {
            return err
        }
        defer f.Close()

        err = tmpl.Execute(f, data)
        if err != nil {
            return err
        }
    }

    return nil
}
