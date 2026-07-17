# A Perfectly Adequate Image Gallery Generator

A small, dependency-light static image gallery generator written in Go. Drop a
folder of images in, run it, and out comes a ready-to-upload gallery — paginated
HTML, resized full-size images, thumbnails, and a lightbox. No framework, no
database, no build step beyond the Go toolchain.

## A little history

This project has a weird pedigree. It was written about three years ago using
Busy3, an agent system I'd written and used myself but never
published — which as far as I can tell makes this some of the oldest
publicly available agentic code anyone has
posted. The code itself was, I'm fairly sure, composed by GPT-3.5 — though I'll be honest, I
don't remember that with total certainty. It was a different era.

I had entirely forgotten it existed until someone mentioned they were using it
in a production project. I looked at the commit dates and my heart sank a
little… and then I actually read it, and it wasn't nearly as bad as I'd braced
for. The bones were fine. It mostly just needed someone to run it twice.

So this is that update: a cleanup pass that fixes the bugs that bite, modernizes
the things that had aged, and leaves the dog-simple spirit intact.

## What it does

- Reads every image in `input/`
- Generates a resized full-size version (fit within `fullSizeWidth`²) and a
  square thumbnail for each
- Emits paginated `galleryN.html` pages with a lightbox, plus an `index.html`
  mirroring the first page
- Honors EXIF orientation so phone photos come out upright, keeps PNGs lossless
  so transparency survives, and writes deterministic filenames so re-running
  doesn't break old links

## Requirements

- Go 1.25 or newer

## Install

```sh
go install github.com/LynnColeArt/AdequateGalleryGenerator@latest
```

Or clone and build a binary yourself:

```sh
git clone https://github.com/LynnColeArt/AdequateGalleryGenerator.git
cd AdequateGalleryGenerator
go build -o gallery
```

## Usage

Throw some images in `input/`, then:

```sh
go run main.go
```

Output appears in `output/` — `gallery1.html`, `gallery2.html`, … and a copy of
the first page as `index.html`. Ready to upload.

### Config

Everything is driven by `config.json`:

| field           | default          | notes                                                |
| --------------- | ---------------- | ---------------------------------------------------- |
| `imagesPerPage` | `12`             | images per generated page                            |
| `fullSizeWidth` | `1500`           | max edge for full-size images (fit within a square)  |
| `thumbnailSize` | `400`            | thumbnail edge length                                |
| `jpegQuality`   | `95`             | JPEG quality (1–100); ignored for PNG output         |
| `inputDir`      | `./input`        | where to read source images                          |
| `outputDir`     | `./output`       | where to write the gallery                           |
| `templatePath`  | `gallery.gohtml` | the HTML template                                    |

Point at a different config with `-config`:

```sh
go run main.go -config /path/to/my-config.json
```

### Notes

- **Output is rebuilt from scratch each run** — `output/` is wiped first, so
  there are no orphaned files from previous runs.
- **PNGs stay lossless** (transparency survives); other formats flatten to JPEG.
- **EXIF orientation is honored**, so phone photos appear upright.
- **Filenames are deterministic** — re-running keeps the same output names, so
  external links survive regeneration.

## Customizing

There's one template, `gallery.gohtml`, and it's barely there — if you know a
little HTML, CSS, and JavaScript you can personalize it without much trouble.

## License

[MIT](LICENSE)

Take care,
--Lynn
