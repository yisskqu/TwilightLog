package media

import (
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const watermarkText = "TwilightLog"

var (
	watermarkFontOnce sync.Once
	watermarkFont     *opentype.Font
)

// applyWatermark draws the temporary TwilightLog mark in the bottom-right corner.
// The mark occupies 5-7% of the long edge, clamped to 24-120 px, at 65% opacity.
func applyWatermark(img *image.NRGBA, longEdge int) {
	if img == nil || longEdge <= 0 {
		return
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 48 || height < 48 {
		return
	}

	target := int(math.Round(float64(longEdge) * 0.06))
	target = clampInt(target, 24, 120)
	if target > width/3 {
		target = width / 3
	}
	if target < 16 {
		return
	}

	mask := buildWatermark(target, longEdge >= 960)
	if mask == nil {
		return
	}

	margin := int(math.Round(float64(minInt(width, height)) * 0.025))
	margin = clampInt(margin, 6, 40)
	x := width - mask.Bounds().Dx() - margin
	y := height - mask.Bounds().Dy() - margin
	if x < 0 || y < 0 {
		return
	}

	blendMask(img, mask, image.Pt(x+1, y+1), color.NRGBA{R: 0, G: 0, B: 0, A: 255}, 0.28)
	blendMask(img, mask, image.Pt(x, y), color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 0.65)
}

// buildWatermark returns an alpha mask whose width fits within targetWidth.
func buildWatermark(targetWidth int, full bool) *image.Alpha {
	if full && targetWidth >= 44 {
		return buildFullWatermark(targetWidth)
	}
	diameter := clampInt(targetWidth, 16, 120)
	return crescentMask(diameter)
}

func buildFullWatermark(targetWidth int) *image.Alpha {
	if watermarkFont == nil {
		watermarkFontOnce.Do(func() {
			parsed, err := opentype.Parse(gobold.TTF)
			if err == nil {
				watermarkFont = parsed
			}
		})
	}
	if watermarkFont == nil {
		return crescentMask(targetWidth)
	}

	markSize := int(math.Round(float64(targetWidth) * 0.34))
	markSize = clampInt(markSize, 14, 48)
	gap := clampInt(int(math.Round(float64(targetWidth)*0.05)), 2, 6)

	fontSize := int(math.Round(float64(markSize) * 0.95))
	textWidth := 0
	var face font.Face
	for fontSize >= 7 {
		candidate, err := opentype.NewFace(watermarkFont, &opentype.FaceOptions{
			Size:    float64(fontSize),
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			break
		}
		measured := font.MeasureString(candidate, watermarkText).Ceil()
		if markSize+gap+measured <= targetWidth {
			face = candidate
			textWidth = measured
			break
		}
		_ = candidate.Close()
		fontSize--
	}
	if face == nil || textWidth == 0 {
		return crescentMask(targetWidth)
	}
	defer face.Close()

	metrics := face.Metrics()
	textHeight := metrics.Ascent.Ceil() + metrics.Descent.Ceil()
	height := maxInt(markSize, textHeight)
	width := markSize + gap + textWidth

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	mark := crescentMask(markSize)
	draw := drawerToAlpha{mask}
	draw.copyAlpha(mark, image.Pt(0, (height-markSize)/2))

	baseline := (height-textHeight)/2 + metrics.Ascent.Ceil()
	drawer := font.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(color.Alpha{A: 255}),
		Face: face,
		Dot:  fixed.P(markSize+gap, baseline),
	}
	drawer.DrawString(watermarkText)
	return mask
}

func crescentMask(size int) *image.Alpha {
	size = clampInt(size, 12, 160)
	const supersample = 4
	bigSize := size * supersample
	big := image.NewAlpha(image.Rect(0, 0, bigSize, bigSize))
	radius := float64(bigSize) * 0.5
	cx := radius
	cy := radius
	innerRadius := radius * 0.82
	innerCX := cx + radius*0.34
	innerCY := cy - radius*0.16

	for y := 0; y < bigSize; y++ {
		for x := 0; x < bigSize; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			dx := px - cx
			dy := py - cy
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			ix := px - innerCX
			iy := py - innerCY
			if ix*ix+iy*iy <= innerRadius*innerRadius {
				continue
			}
			big.SetAlpha(x, y, color.Alpha{A: 255})
		}
	}

	small := image.NewAlpha(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sum := 0
			for dy := 0; dy < supersample; dy++ {
				for dx := 0; dx < supersample; dx++ {
					sum += int(big.AlphaAt(x*supersample+dx, y*supersample+dy).A)
				}
			}
			small.SetAlpha(x, y, color.Alpha{A: uint8(sum / (supersample * supersample))})
		}
	}
	return small
}

type drawerToAlpha struct {
	dst *image.Alpha
}

func (d drawerToAlpha) copyAlpha(src *image.Alpha, at image.Point) {
	bounds := src.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			alpha := src.AlphaAt(x, y).A
			if alpha == 0 {
				continue
			}
			px := at.X + x - bounds.Min.X
			py := at.Y + y - bounds.Min.Y
			if !image.Pt(px, py).In(d.dst.Bounds()) {
				continue
			}
			d.dst.SetAlpha(px, py, color.Alpha{A: alpha})
		}
	}
}

func blendMask(dst *image.NRGBA, mask *image.Alpha, at image.Point, tint color.NRGBA, opacity float64) {
	maskBounds := mask.Bounds()
	dstBounds := dst.Bounds()
	for y := maskBounds.Min.Y; y < maskBounds.Max.Y; y++ {
		for x := maskBounds.Min.X; x < maskBounds.Max.X; x++ {
			alpha := mask.AlphaAt(x, y).A
			if alpha == 0 {
				continue
			}
			px := at.X + x - maskBounds.Min.X
			py := at.Y + y - maskBounds.Min.Y
			if !image.Pt(px, py).In(dstBounds) {
				continue
			}
			coverage := float64(alpha) / 255.0 * opacity
			i := dst.PixOffset(px, py)
			r := float64(dst.Pix[i])
			g := float64(dst.Pix[i+1])
			b := float64(dst.Pix[i+2])
			dst.Pix[i] = uint8(clampFloat(r+(float64(tint.R)-r)*coverage, 0, 255))
			dst.Pix[i+1] = uint8(clampFloat(g+(float64(tint.G)-g)*coverage, 0, 255))
			dst.Pix[i+2] = uint8(clampFloat(b+(float64(tint.B)-b)*coverage, 0, 255))
		}
	}
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
