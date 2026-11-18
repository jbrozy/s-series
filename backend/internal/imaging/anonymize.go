package imaging

import (
	"image"
	"math"
	"sort"
)

// Style of the silhouette rendering (technical line art look).
var (
	silhouetteFill = [3]uint8{214, 219, 226}
	silhouetteLine = [3]uint8{62, 72, 86}
	solidFill      = [3]uint8{128, 128, 128}
)

// Anonymize applies all detections to a copy of the image.
func Anonymize(src image.Image, dets []Detection) *image.RGBA {
	img := ToRGBA(src)
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	for _, d := range dets {
		b := ClampBox(d.Box, W, H)
		if b[2]-b[0] < 2 || b[3]-b[1] < 2 {
			continue
		}
		mode := d.Mode
		if mode == "" {
			mode = DefaultMode(d.Label)
		}
		switch mode {
		case "silhouette":
			silhouette(img, b)
		case "outline":
			outline(img, b)
		case "pixelate":
			pixelate(img, b)
		case "blur":
			blurRegion(img, b)
		case "solid":
			fill(img, b, solidFill)
		}
	}
	return img
}

type region struct {
	x0, y0, w, h int
}

func regionOf(b [4]int, pad, W, H int) region {
	x0, y0 := max(0, b[0]-pad), max(0, b[1]-pad)
	x1, y1 := min(W, b[2]+pad), min(H, b[3]+pad)
	return region{x0, y0, x1 - x0, y1 - y0}
}

func (r region) rgb(img *image.RGBA) [][3]float64 {
	out := make([][3]float64, r.w*r.h)
	for y := 0; y < r.h; y++ {
		o := (r.y0+y)*img.Stride + r.x0*4
		for x := 0; x < r.w; x++ {
			out[y*r.w+x] = [3]float64{float64(img.Pix[o]), float64(img.Pix[o+1]), float64(img.Pix[o+2])}
			o += 4
		}
	}
	return out
}

func set(img *image.RGBA, x, y int, c [3]uint8) {
	o := y*img.Stride + x*4
	img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c[0], c[1], c[2], 255
}

func fill(img *image.RGBA, b [4]int, c [3]uint8) {
	for y := b[1]; y < b[3]; y++ {
		for x := b[0]; x < b[2]; x++ {
			set(img, x, y, c)
		}
	}
}

// boxBlur blurs a float field (single channel) with radius r (separable).
func boxBlur(src []float64, w, h, r int) []float64 {
	if r <= 0 {
		return src
	}
	tmp := make([]float64, len(src))
	out := make([]float64, len(src))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s, n := 0.0, 0
			for k := max(0, x-r); k <= min(w-1, x+r); k++ {
				s += src[y*w+k]
				n++
			}
			tmp[y*w+x] = s / float64(n)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s, n := 0.0, 0
			for k := max(0, y-r); k <= min(h-1, y+r); k++ {
				s += tmp[k*w+x]
				n++
			}
			out[y*w+x] = s / float64(n)
		}
	}
	return out
}

func sobel(g []float64, w, h int) []float64 {
	out := make([]float64, len(g))
	at := func(x, y int) float64 {
		x, y = max(0, min(w-1, x)), max(0, min(h-1, y))
		return g[y*w+x]
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx := -at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1) + at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1)
			gy := -at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1) + at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1)
			out[y*w+x] = math.Hypot(gx, gy)
		}
	}
	return out
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	c := append([]float64(nil), vals...)
	sort.Float64s(c)
	i := int(p * float64(len(c)-1))
	return c[i]
}

// morph performs square dilation (dilate=true) or erosion with radius r.
func morph(m []bool, w, h, r int, dilate bool) []bool {
	if r <= 0 {
		return m
	}
	tmp := make([]bool, len(m))
	out := make([]bool, len(m))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := !dilate
			for k := max(0, x-r); k <= min(w-1, x+r); k++ {
				if dilate && m[y*w+k] {
					v = true
					break
				}
				if !dilate && !m[y*w+k] {
					v = false
					break
				}
			}
			tmp[y*w+x] = v
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := !dilate
			for k := max(0, y-r); k <= min(h-1, y+r); k++ {
				if dilate && tmp[k*w+x] {
					v = true
					break
				}
				if !dilate && !tmp[k*w+x] {
					v = false
					break
				}
			}
			out[y*w+x] = v
		}
	}
	return out
}

// fillHoles marks background areas not connected to the region border as foreground.
func fillHoles(m []bool, w, h int) []bool {
	reach := make([]bool, len(m))
	stack := []int{}
	push := func(x, y int) {
		i := y*w + x
		if !m[i] && !reach[i] {
			reach[i] = true
			stack = append(stack, i)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p%w, p/w
		if x > 0 {
			push(x-1, y)
		}
		if x < w-1 {
			push(x+1, y)
		}
		if y > 0 {
			push(x, y-1)
		}
		if y < h-1 {
			push(x, y+1)
		}
	}
	out := make([]bool, len(m))
	for i := range m {
		out[i] = m[i] || !reach[i]
	}
	return out
}

// silhouette replaces the body part by a flat, line-art style silhouette:
// shape and main contours (fingers) stay recognizable, skin colour,
// texture, jewellery, tattoos and fingerprints disappear.
func silhouette(img *image.RGBA, b [4]int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	r := regionOf(b, 3, W, H)
	if r.w < 6 || r.h < 6 {
		pixelate(img, b)
		return
	}
	px := r.rgb(img)
	ch := channels(px, r.w, r.h, 1)
	n := r.w * r.h
	bg := ringColor(ch, r.w, r.h)

	// 1) photos: skin colour prior
	mask := make([]bool, n)
	skin := 0
	for i := 0; i < n; i++ {
		if isSkin(ch[0][i], ch[1][i], ch[2][i]) {
			mask[i] = true
			skin++
		}
	}
	if float64(skin) < 0.06*float64(n) {
		// 2) gloves / drawings: difference to the dominant background colours
		centers := dominantColors(ch, r.w, r.h)
		for i := 0; i < n; i++ {
			best := math.MaxFloat64
			for _, c := range centers {
				best = math.Min(best, math.Sqrt(sq(ch[0][i]-c[0])+sq(ch[1][i]-c[1])+sq(ch[2][i]-c[2])))
			}
			mask[i] = best > 32
		}
	}
	mask = morph(morph(mask, r.w, r.h, 2, true), r.w, r.h, 2, false) // close
	mask = morph(morph(mask, r.w, r.h, 1, false), r.w, r.h, 1, true) // open
	mask = fillHoles(mask, r.w, r.h)
	minArea := int(0.015 * float64(n))
	keep := make([]bool, n)
	area := 0
	for _, c := range componentsIdx(mask, r.w, r.h) {
		if len(c) >= minArea {
			for _, i := range c {
				keep[i] = true
			}
			area += len(c)
		}
	}
	frac := float64(area) / float64(n)
	if frac < 0.05 || frac > 0.97 {
		outline(img, b)
		return
	}
	edges := colorSobel(channels(px, r.w, r.h, 2), r.w, r.h)
	var inside []float64
	for i, k := range keep {
		if k {
			inside = append(inside, edges[i])
		}
	}
	et := math.Max(18, percentile(inside, 0.9))
	border := make([]bool, n)
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			i := y*r.w + x
			if !keep[i] {
				continue
			}
			if x == 0 || y == 0 || x == r.w-1 || y == r.h-1 || !keep[i-1] || !keep[i+1] || !keep[i-r.w] || !keep[i+r.w] {
				border[i] = true
			}
		}
	}
	// do not draw a contour where the body part leaves the box (arm continues)
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			if x <= 1 || y <= 1 || x >= r.w-2 || y >= r.h-2 {
				border[y*r.w+x] = false
			}
		}
	}
	border = morph(border, r.w, r.h, 1, true)
	fillC := silhouetteFill
	if luminance(bg) > 200 { // light backgrounds: slightly darker fill
		fillC = [3]uint8{196, 203, 212}
	}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			i := y*r.w + x
			if !keep[i] && !border[i] {
				continue
			}
			c := fillC
			if border[i] || (keep[i] && edges[i] > et) {
				c = silhouetteLine
			}
			set(img, r.x0+x, r.y0+y, c)
		}
	}
}

func luminance(c [3]float64) float64 { return 0.299*c[0] + 0.587*c[1] + 0.114*c[2] }

// isSkin is a broad YCbCr skin classifier (all skin tones).
func isSkin(r, g, b float64) bool {
	y := 0.299*r + 0.587*g + 0.114*b
	cb := 128 - 0.168736*r - 0.331264*g + 0.5*b
	cr := 128 + 0.5*r - 0.418688*g - 0.081312*b
	return y > 40 && cb >= 80 && cb <= 135 && cr >= 135 && cr <= 180 && r > g && r > b
}

func channels(px [][3]float64, w, h, blur int) [3][]float64 {
	var ch [3][]float64
	for c := 0; c < 3; c++ {
		ch[c] = make([]float64, len(px))
		for i := range px {
			ch[c][i] = px[i][c]
		}
		ch[c] = boxBlur(ch[c], w, h, blur)
	}
	return ch
}

func colorSobel(ch [3][]float64, w, h int) []float64 {
	out := make([]float64, w*h)
	for c := 0; c < 3; c++ {
		e := sobel(ch[c], w, h)
		for i := range out {
			out[i] = math.Max(out[i], e[i])
		}
	}
	return out
}

func ringIndices(w, h int) []int {
	t := max(1, min(3, min(w, h)/8))
	var idx []int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < t || y < t || x >= w-t || y >= h-t {
				idx = append(idx, y*w+x)
			}
		}
	}
	return idx
}

func ringColor(ch [3][]float64, w, h int) [3]float64 {
	var bg [3]float64
	idx := ringIndices(w, h)
	for c := 0; c < 3; c++ {
		vals := make([]float64, len(idx))
		for k, i := range idx {
			vals[k] = ch[c][i]
		}
		bg[c] = percentile(vals, 0.5)
	}
	return bg
}

// dominantColors clusters the border ring (k-means, k<=4) and returns the
// clusters that cover at least 15% of the ring: these are background.
func dominantColors(ch [3][]float64, w, h int) [][3]float64 {
	idx := ringIndices(w, h)
	k := 4
	centers := make([][3]float64, k)
	for j := 0; j < k; j++ {
		i := idx[(j*len(idx))/k]
		centers[j] = [3]float64{ch[0][i], ch[1][i], ch[2][i]}
	}
	counts := make([]int, k)
	for iter := 0; iter < 8; iter++ {
		sums := make([][3]float64, k)
		for j := range counts {
			counts[j] = 0
		}
		for _, i := range idx {
			best, bd := 0, math.MaxFloat64
			for j, c := range centers {
				d := sq(ch[0][i]-c[0]) + sq(ch[1][i]-c[1]) + sq(ch[2][i]-c[2])
				if d < bd {
					best, bd = j, d
				}
			}
			counts[best]++
			for c := 0; c < 3; c++ {
				sums[best][c] += ch[c][i]
			}
		}
		for j := range centers {
			if counts[j] > 0 {
				for c := 0; c < 3; c++ {
					centers[j][c] = sums[j][c] / float64(counts[j])
				}
			}
		}
	}
	var out [][3]float64
	for j, c := range centers {
		if float64(counts[j]) >= 0.15*float64(len(idx)) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = append(out, ringColor(ch, w, h))
	}
	return out
}

func sq(v float64) float64 { return v * v }

func componentsIdx(mask []bool, w, h int) [][]int {
	seen := make([]bool, len(mask))
	var out [][]int
	for i := range mask {
		if !mask[i] || seen[i] {
			continue
		}
		var comp []int
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			comp = append(comp, p)
			x, y := p%w, p/w
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				q := ny*w + nx
				if mask[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		out = append(out, comp)
	}
	return out
}

// outline turns the region into a line drawing on the surrounding
// background colour.
func outline(img *image.RGBA, b [4]int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	r := regionOf(b, 0, W, H)
	if r.w < 4 || r.h < 4 {
		pixelate(img, b)
		return
	}
	ch := channels(r.rgb(img), r.w, r.h, 2)
	bg := ringColor(ch, r.w, r.h)
	e := colorSobel(ch, r.w, r.h)
	et := math.Max(14, percentile(e, 0.85))
	bgc := [3]uint8{uint8(bg[0]), uint8(bg[1]), uint8(bg[2])}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			c := bgc
			if e[y*r.w+x] > et {
				c = silhouetteLine
			}
			set(img, r.x0+x, r.y0+y, c)
		}
	}
}

func pixelate(img *image.RGBA, b [4]int) {
	w, h := b[2]-b[0], b[3]-b[1]
	bs := max(6, min(w, h)/8)
	for by := b[1]; by < b[3]; by += bs {
		for bx := b[0]; bx < b[2]; bx += bs {
			var s [3]int
			n := 0
			for y := by; y < min(by+bs, b[3]); y++ {
				for x := bx; x < min(bx+bs, b[2]); x++ {
					o := y*img.Stride + x*4
					s[0] += int(img.Pix[o])
					s[1] += int(img.Pix[o+1])
					s[2] += int(img.Pix[o+2])
					n++
				}
			}
			c := [3]uint8{uint8(s[0] / n), uint8(s[1] / n), uint8(s[2] / n)}
			for y := by; y < min(by+bs, b[3]); y++ {
				for x := bx; x < min(bx+bs, b[2]); x++ {
					set(img, x, y, c)
				}
			}
		}
	}
}

func blurRegion(img *image.RGBA, b [4]int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	r := regionOf(b, 0, W, H)
	px := r.rgb(img)
	rad := max(4, min(r.w, r.h)/10)
	var ch [3][]float64
	for c := 0; c < 3; c++ {
		ch[c] = make([]float64, len(px))
		for i := range px {
			ch[c][i] = px[i][c]
		}
		for pass := 0; pass < 3; pass++ {
			ch[c] = boxBlur(ch[c], r.w, r.h, rad)
		}
	}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			i := y*r.w + x
			set(img, r.x0+x, r.y0+y, [3]uint8{uint8(ch[0][i]), uint8(ch[1][i]), uint8(ch[2][i])})
		}
	}
}
