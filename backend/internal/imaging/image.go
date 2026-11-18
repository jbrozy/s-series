// Package imaging detects human body parts in illustrations and makes them
// unrecognizable while keeping their meaning (a hand stays recognizable as
// a hand, but loses skin, texture and identifying features).
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// Detection is one detected (or manually drawn) body part region.
type Detection struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Box    [4]int  `json:"box"` // x1, y1, x2, y2 in pixels of the original image
	Score  float64 `json:"score"`
	Source string  `json:"source"` // auto | manual
	Mode   string  `json:"mode"`   // silhouette | outline | pixelate | blur | solid | none
}

// Modes available for anonymization.
var Modes = []string{"silhouette", "outline", "pixelate", "blur", "solid", "none"}

// DefaultMode chooses the anonymization per body part: faces must be
// unrecognizable, extremities keep their shape.
func DefaultMode(label string) string {
	switch NormalizeLabel(label) {
	case "face", "head":
		return "pixelate"
	case "skin":
		return "silhouette"
	}
	return "silhouette"
}

var labelMap = map[string]string{
	"hand": "hand", "hands": "hand", "palm": "hand", "hand_left": "hand", "hand_right": "hand", "left hand": "hand", "right hand": "hand",
	"finger": "finger", "fingers": "finger", "thumb": "finger", "fingertip": "finger",
	"face": "face", "faces": "face", "eye": "face", "eyes": "face", "mouth": "face", "nose": "face",
	"head": "head", "hair": "head", "ear": "head",
	"arm": "arm", "arms": "arm", "forearm": "arm", "wrist": "arm", "elbow": "arm",
	"leg": "leg", "legs": "leg", "knee": "leg",
	"foot": "foot", "feet": "foot",
	"torso": "torso", "body": "torso", "upper body": "torso",
	"person": "person", "people": "person", "human": "person", "man": "person", "woman": "person", "worker": "person", "technician": "person",
	"skin":    "skin",
	"gesicht": "face", "kopf": "head", "bein": "leg", "fuß": "foot", "fuss": "foot", "hände": "hand",
}

// NormalizeLabel maps model output labels to a canonical set.
func NormalizeLabel(l string) string {
	k := strings.ToLower(strings.TrimSpace(l))
	if v, ok := labelMap[k]; ok {
		return v
	}
	for _, key := range []string{"hand", "finger", "face", "head", "foot", "person", "torso"} {
		if strings.Contains(k, key) {
			return key
		}
	}
	return k
}

// LabelDE returns a German display label.
func LabelDE(l string) string {
	switch NormalizeLabel(l) {
	case "hand":
		return "Hand"
	case "finger":
		return "Finger"
	case "face":
		return "Gesicht"
	case "head":
		return "Kopf"
	case "arm":
		return "Arm"
	case "leg":
		return "Bein"
	case "foot":
		return "Fuß"
	case "torso":
		return "Oberkörper"
	case "person":
		return "Person"
	case "skin":
		return "Hautbereich"
	}
	return l
}

// Load decodes an image file. format is png, jpeg, gif, tiff or bmp.
func Load(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return Decode(f, path)
}

func Decode(r io.Reader, name string) (image.Image, string, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		img, err := png.Decode(r)
		return img, "png", err
	case ".jpg", ".jpeg":
		img, err := jpeg.Decode(r)
		return img, "jpeg", err
	case ".gif":
		img, err := gif.Decode(r)
		return img, "gif", err
	case ".tif", ".tiff":
		img, err := tiff.Decode(r)
		return img, "tiff", err
	case ".bmp":
		img, err := bmp.Decode(r)
		return img, "bmp", err
	}
	return nil, "", fmt.Errorf("Bildformat %s wird nicht unterstützt", filepath.Ext(name))
}

// Encode writes the image in the given format.
func Encode(w io.Writer, img image.Image, format string) error {
	switch format {
	case "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
	case "gif":
		return gif.Encode(w, img, nil)
	case "tiff":
		return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate})
	case "bmp":
		return bmp.Encode(w, img)
	}
	return png.Encode(w, img)
}

// MimeFor returns the MIME type of a format.
func MimeFor(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "tiff":
		return "image/tiff"
	case "bmp":
		return "image/bmp"
	}
	return "image/png"
}

// ToRGBA copies an image into a fresh RGBA with origin (0,0).
func ToRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	// flatten transparency onto white (technical illustrations)
	for i := 0; i < len(out.Pix); i += 4 {
		a := out.Pix[i+3]
		if a < 255 {
			f := float64(a) / 255
			for c := 0; c < 3; c++ {
				out.Pix[i+c] = uint8(float64(out.Pix[i+c]) + (1-f)*255)
			}
			out.Pix[i+3] = 255
		}
	}
	return out
}

// Downscale reduces the image so that its longest side is at most maxSide
// (area averaging). It returns the scale factor original/new.
func Downscale(src *image.RGBA, maxSide int) (*image.RGBA, float64) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if maxSide <= 0 || (w <= maxSide && h <= maxSide) {
		return src, 1
	}
	scale := float64(max(w, h)) / float64(maxSide)
	nw, nh := max(1, int(float64(w)/scale)), max(1, int(float64(h)/scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := int(float64(y)*scale), min(h, int(float64(y+1)*scale)+1)
		for x := 0; x < nw; x++ {
			x0, x1 := int(float64(x)*scale), min(w, int(float64(x+1)*scale)+1)
			var r, g, b, n int
			for yy := y0; yy < y1; yy++ {
				off := yy*src.Stride + x0*4
				for xx := x0; xx < x1; xx++ {
					r += int(src.Pix[off])
					g += int(src.Pix[off+1])
					b += int(src.Pix[off+2])
					n++
					off += 4
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
		}
	}
	return dst, scale
}

// PNGBytes encodes an image as PNG.
func PNGBytes(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ClampBox keeps a box inside the image and orders the coordinates.
func ClampBox(b [4]int, w, h int) [4]int {
	x1, y1, x2, y2 := b[0], b[1], b[2], b[3]
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	x1, y1 = max(0, min(x1, w)), max(0, min(y1, h))
	x2, y2 = max(0, min(x2, w)), max(0, min(y2, h))
	return [4]int{x1, y1, x2, y2}
}

// Size reads only the image header and returns width and height.
func Size(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var cfg image.Config
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		cfg, err = png.DecodeConfig(f)
	case ".jpg", ".jpeg":
		cfg, err = jpeg.DecodeConfig(f)
	case ".gif":
		cfg, err = gif.DecodeConfig(f)
	case ".tif", ".tiff":
		cfg, err = tiff.DecodeConfig(f)
	case ".bmp":
		cfg, err = bmp.DecodeConfig(f)
	default:
		return 0, 0, fmt.Errorf("Bildformat %s wird nicht unterstützt", filepath.Ext(path))
	}
	return cfg.Width, cfg.Height, err
}
