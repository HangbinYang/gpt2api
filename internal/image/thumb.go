package image

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

const (
	// DefaultThumbMaxBytes 兼容旧调用方的默认预算。
	DefaultThumbMaxBytes = 10 * 1024
	// MaxThumbKB 单次缩略图请求允许的最大体积。
	MaxThumbKB = 64
)

type thumbStage struct {
	MaxWidth int
	Quality  int
}

var thumbStages = []thumbStage{
	{MaxWidth: 768, Quality: 78},
	{MaxWidth: 640, Quality: 70},
	{MaxWidth: 512, Quality: 62},
	{MaxWidth: 384, Quality: 55},
	{MaxWidth: 256, Quality: 50},
	{MaxWidth: 192, Quality: 45},
}

// ClampThumbKB 把外部传入的 KB 数夹到 [0, MaxThumbKB]。
func ClampThumbKB(kb int) int {
	if kb <= 0 {
		return 0
	}
	if kb > MaxThumbKB {
		return MaxThumbKB
	}
	return kb
}

// MakeThumbnail 将任意主流格式压缩为 JPEG 缩略图。
func MakeThumbnail(src []byte, budgetKB int) ([]byte, string, bool) {
	budgetKB = ClampThumbKB(budgetKB)
	if budgetKB <= 0 || len(src) == 0 {
		return nil, "", false
	}
	budget := budgetKB * 1024

	srcImg, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", false
	}
	b := srcImg.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, "", false
	}

	if len(src) <= budget {
		if out, ok := encodeThumbStage(srcImg, sw, sh, thumbStages[0]); ok && len(out) <= len(src) {
			return out, "image/jpeg", true
		}
		return nil, "", false
	}

	var last []byte
	for _, st := range thumbStages {
		out, ok := encodeThumbStage(srcImg, sw, sh, st)
		if !ok {
			continue
		}
		last = out
		if len(out) <= budget {
			return out, "image/jpeg", true
		}
	}
	if len(last) > 0 {
		return last, "image/jpeg", true
	}
	return nil, "", false
}

// MakeThumbJPEG 兼容旧接口:按 maxBytes 预算生成 JPEG 缩略图。
func MakeThumbJPEG(srcBytes []byte, maxBytes int) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultThumbMaxBytes
	}
	kb := (maxBytes + 1023) / 1024
	out, ct, ok := MakeThumbnail(srcBytes, kb)
	if !ok {
		return nil, "", errors.New("thumb encode failed")
	}
	if len(out) > maxBytes {
		return out, ct, nil
	}
	return out, ct, nil
}

func encodeThumbStage(srcImg image.Image, sw, sh int, st thumbStage) ([]byte, bool) {
	q := st.Quality
	if q < 1 {
		q = 1
	}
	if q > 100 {
		q = 100
	}

	target := st.MaxWidth
	long := sw
	if sh > long {
		long = sh
	}

	var dst image.Image
	if long <= target {
		dst = srcImg
	} else {
		var dw, dh int
		if sw >= sh {
			dw = target
			dh = int(float64(sh) * float64(target) / float64(sw))
		} else {
			dh = target
			dw = int(float64(sw) * float64(target) / float64(sh))
		}
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
		canvas := image.NewRGBA(image.Rect(0, 0, dw, dh))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), srcImg, srcImg.Bounds(), draw.Src, nil)
		dst = canvas
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: q}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}
