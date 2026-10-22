package imagemeta

import "image"

// orient returns img transformed the way an Exif Orientation value asks a
// viewer to display it, so the result is upright with no orientation tag.
// Luma and chroma are copied sample for sample; only a source that is neither
// YCbCr nor grey (CMYK) is converted to RGBA.
func orient(img image.Image, o byte) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	source := sourcePixel(o, width, height)
	rect := image.Rect(0, 0, width, height)
	if o >= 5 {
		rect = image.Rect(0, 0, height, width)
	}
	switch src := img.(type) {
	case *image.YCbCr:
		dst := image.NewYCbCr(rect, image.YCbCrSubsampleRatio444)
		for y := range rect.Dy() {
			for x := range rect.Dx() {
				sx, sy := source(x, y)
				sx, sy = sx+bounds.Min.X, sy+bounds.Min.Y
				c := src.COffset(sx, sy)
				dst.Y[y*dst.YStride+x] = src.Y[src.YOffset(sx, sy)]
				dst.Cb[y*dst.CStride+x] = src.Cb[c]
				dst.Cr[y*dst.CStride+x] = src.Cr[c]
			}
		}
		return dst
	case *image.Gray:
		dst := image.NewGray(rect)
		for y := range rect.Dy() {
			for x := range rect.Dx() {
				sx, sy := source(x, y)
				dst.Pix[y*dst.Stride+x] = src.Pix[src.PixOffset(sx+bounds.Min.X, sy+bounds.Min.Y)]
			}
		}
		return dst
	default:
		dst := image.NewRGBA(rect)
		for y := range rect.Dy() {
			for x := range rect.Dx() {
				sx, sy := source(x, y)
				dst.Set(x, y, img.At(sx+bounds.Min.X, sy+bounds.Min.Y))
			}
		}
		return dst
	}
}

// sourcePixel maps a pixel of the upright image back to the stored image of
// the given width and height, for each Exif Orientation value.
func sourcePixel(o byte, width, height int) func(x, y int) (int, int) {
	switch o {
	case 2: // mirrored horizontally
		return func(x, y int) (int, int) { return width - 1 - x, y }
	case 3: // rotated 180°
		return func(x, y int) (int, int) { return width - 1 - x, height - 1 - y }
	case 4: // mirrored vertically
		return func(x, y int) (int, int) { return x, height - 1 - y }
	case 5: // transposed
		return func(x, y int) (int, int) { return y, x }
	case 6: // needs a 90° clockwise turn
		return func(x, y int) (int, int) { return y, height - 1 - x }
	case 7: // transversed
		return func(x, y int) (int, int) { return width - 1 - y, height - 1 - x }
	case 8: // needs a 90° counter-clockwise turn
		return func(x, y int) (int, int) { return width - 1 - y, x }
	default:
		return func(x, y int) (int, int) { return x, y }
	}
}
