package media

import (
	"image"
	"testing"
)

func TestApplyWatermarkOnlyTouchesBottomRight(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1600, 1067))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 120
		img.Pix[i+1] = 120
		img.Pix[i+2] = 120
		img.Pix[i+3] = 255
	}

	applyWatermark(img, 1600)

	changedBottomRight := 0
	changedTopLeft := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			offset := img.PixOffset(x, y)
			if img.Pix[offset] == 120 && img.Pix[offset+1] == 120 && img.Pix[offset+2] == 120 {
				continue
			}
			if x > img.Bounds().Dx()*3/4 && y > img.Bounds().Dy()*3/4 {
				changedBottomRight++
			} else if x < img.Bounds().Dx()/4 && y < img.Bounds().Dy()/4 {
				changedTopLeft++
			}
		}
	}
	if changedBottomRight == 0 {
		t.Fatal("watermark did not change the bottom-right corner")
	}
	if changedTopLeft != 0 {
		t.Fatalf("watermark leaked into the top-left corner: %d pixels", changedTopLeft)
	}
}

func TestBuildWatermarkFitsTargetWidth(t *testing.T) {
	for _, target := range []int{24, 40, 58, 96, 120} {
		for _, full := range []bool{false, true} {
			mask := buildWatermark(target, full)
			if mask == nil {
				t.Fatalf("nil mask for target %d full=%v", target, full)
			}
			if mask.Bounds().Dx() > target {
				t.Fatalf("mask width %d exceeds target %d (full=%v)", mask.Bounds().Dx(), target, full)
			}
		}
	}
}

func TestApplyWatermarkSkipsTinyImages(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 20
		img.Pix[i+1] = 20
		img.Pix[i+2] = 20
		img.Pix[i+3] = 255
	}
	applyWatermark(img, 40)
	if img.Pix[0] != 20 || img.Pix[1] != 20 || img.Pix[2] != 20 {
		t.Fatal("tiny image should not be watermarked")
	}
}

func TestCrescentMaskHasCoverage(t *testing.T) {
	mask := crescentMask(32)
	covered := 0
	for _, alpha := range mask.Pix {
		if alpha > 0 {
			covered++
		}
	}
	if covered == 0 {
		t.Fatal("crescent mask is empty")
	}
	if covered == len(mask.Pix) {
		t.Fatal("crescent mask should have transparent pixels")
	}
}
