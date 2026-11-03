package sftp

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
)

const (
	// thumbnailHeadBytes covers an Exif segment, whose length field stops at 64 KiB, and the short segment that may
	// come before it. A camera's photo costs this much rather than the whole file.
	thumbnailHeadBytes = 68 << 10
	// maxThumbnailSourceBytes is the most of one file read to shrink it. Over a phone's mobile connection a screen of
	// about 20 such files costs at most 80 MiB; a photo with a picture in its Exif header is not read this far.
	maxThumbnailSourceBytes = 4 << 20
	// maxThumbnailDecodedBytes is the most memory one decode may use. A 40-megapixel photo fits; a small file that
	// declares more pixels than that (a decompression bomb) is refused before anything is allocated.
	maxThumbnailDecodedBytes = 128 << 20
	// progressiveDecodeFactor is what a progressive JPEG's decoder holds per byte of pixels: its coefficients take
	// 4 bytes per sample until the last scan has been read.
	progressiveDecodeFactor = 5
	// minEmbeddedThumbnailEdge keeps an Exif picture from being drawn when it would be a blur even in a list row.
	minEmbeddedThumbnailEdge = 120
	// thumbnailLongestEdge fills a grid cell at a phone's pixel density without sending the screen more than that.
	thumbnailLongestEdge = 256
	// thumbnailSamplesPerAxis is how many points of the source each thumbnail pixel averages along each axis. A fixed
	// number keeps the time a 40-megapixel source takes the same as a 1-megapixel one's.
	thumbnailSamplesPerAxis = 3
	// thumbnailJPEGQuality keeps a thumbnail near 15 KiB; the screen shows it at a fraction of a phone's width.
	thumbnailJPEGQuality = 72
	// thumbnailReadChunkBytes is how much is read between looks at the cancellation: a row scrolled out of view stops
	// costing data within one chunk.
	thumbnailReadChunkBytes = 256 << 10
)

// Thumbnail is a small picture of a file, ready to send to the screen. ContentType is image/jpeg, or image/png when
// the picture has transparent pixels.
type Thumbnail struct {
	ContentType string
	Contents    []byte
}

// thumbnailSource is what was read to make a thumbnail: a whole file, or the picture in its Exif header.
type thumbnailSource struct {
	contentType string
	encoded     []byte
	orientation int
}

// readThumbnailSource reads no more of a file than its thumbnail needs. The file's bytes decide what it is, never its
// name, and SVG is refused: it is a program for a renderer, not pixels, and the engine does not run one.
func readThumbnailSource(ctx context.Context, file io.Reader, size int64) (thumbnailSource, error) {
	reader := &contextReader{ctx: ctx, reader: file}
	head, err := readHead(reader, nil, previewSniffBytes)
	if err != nil {
		return thumbnailSource{}, err
	}
	contentType := previewContentType(head)
	if !decodableThumbnailType(contentType) {
		return thumbnailSource{}, ErrPreviewType
	}
	orientation := exifUprightOrientation
	if contentType == "image/jpeg" {
		if head, err = readHead(reader, head, thumbnailHeadBytes); err != nil {
			return thumbnailSource{}, err
		}
		exif := readJPEGExif(head)
		orientation = exif.orientation
		if usableEmbeddedThumbnail(exif.embedded) {
			return thumbnailSource{contentType: contentType, encoded: exif.embedded, orientation: orientation}, nil
		}
	}
	if size > maxThumbnailSourceBytes {
		return thumbnailSource{}, ErrPreviewTooLarge
	}
	// A header already in hand can refuse the file before the rest of it costs anything. A header past the head is
	// checked by renderThumbnail once the file has been read, and the size bound above holds until then.
	if config, err := decodeThumbnailConfig(contentType, head); err == nil && decodedBytes(contentType, head, config) > maxThumbnailDecodedBytes {
		return thumbnailSource{}, ErrPreviewTooLarge
	}
	contents, err := readRest(reader, head, size)
	if err != nil {
		return thumbnailSource{}, err
	}
	return thumbnailSource{contentType: contentType, encoded: contents, orientation: orientation}, nil
}

func decodableThumbnailType(contentType string) bool {
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif"
}

func usableEmbeddedThumbnail(embedded []byte) bool {
	if embedded == nil {
		return false
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(embedded))
	return err == nil && max(config.Width, config.Height) >= minEmbeddedThumbnailEdge
}

// readHead reads until head holds want bytes or the file ends.
func readHead(reader io.Reader, head []byte, want int) ([]byte, error) {
	if len(head) >= want {
		return head, nil
	}
	grown := make([]byte, want)
	copy(grown, head)
	read, err := io.ReadFull(reader, grown[len(head):])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return grown[:len(head)+read], nil
}

// readRest reads the file to the size it was listed at, a chunk at a time. A file shorter than that has changed since.
func readRest(reader io.Reader, head []byte, size int64) ([]byte, error) {
	if int64(len(head)) >= size {
		return head, nil
	}
	contents := make([]byte, size)
	filled := copy(contents, head)
	for filled < len(contents) {
		chunk := contents[filled:min(filled+thumbnailReadChunkBytes, len(contents))]
		read, err := io.ReadFull(reader, chunk)
		filled += read
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
	}
	return contents, nil
}

func decodeThumbnailConfig(contentType string, encoded []byte) (image.Config, error) {
	reader := bytes.NewReader(encoded)
	switch contentType {
	case "image/jpeg":
		return jpeg.DecodeConfig(reader)
	case "image/png":
		return png.DecodeConfig(reader)
	case "image/gif":
		return gif.DecodeConfig(reader)
	}
	return image.Config{}, ErrPreviewType
}

// decodedBytes says how much memory decoding this image takes, from its header alone.
func decodedBytes(contentType string, encoded []byte, config image.Config) int64 {
	pixels := int64(config.Width) * int64(config.Height)
	total := pixels * bytesPerPixel(config.ColorModel)
	if contentType == "image/jpeg" && jpegIsProgressive(encoded) {
		return total * progressiveDecodeFactor
	}
	return total
}

func bytesPerPixel(model color.Model) int64 {
	if _, paletted := model.(color.Palette); paletted {
		return 1
	}
	switch model {
	case color.GrayModel:
		return 1
	case color.Gray16Model:
		return 2
	case color.YCbCrModel:
		// Without subsampling, the most a JPEG's three channels can take.
		return 3
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	}
	return 4
}

// renderThumbnail decodes a source and shrinks it. A source that cannot be decoded is refused like any other file
// that has no picture to show; the screen keeps the file's icon.
func renderThumbnail(source thumbnailSource) (Thumbnail, error) {
	config, err := decodeThumbnailConfig(source.contentType, source.encoded)
	if err != nil {
		return Thumbnail{}, ErrPreviewType
	}
	if decodedBytes(source.contentType, source.encoded, config) > maxThumbnailDecodedBytes {
		return Thumbnail{}, ErrPreviewTooLarge
	}
	decoded, err := decodeThumbnailImage(source)
	if err != nil || decoded.Bounds().Empty() {
		return Thumbnail{}, ErrPreviewType
	}
	return encodeThumbnail(shrunkAndUpright(decoded, source.orientation))
}

// decodeThumbnailImage takes the first frame of a GIF; a list has no use for the rest.
func decodeThumbnailImage(source thumbnailSource) (image.Image, error) {
	reader := bytes.NewReader(source.encoded)
	switch source.contentType {
	case "image/jpeg":
		return jpeg.Decode(reader)
	case "image/png":
		return png.Decode(reader)
	case "image/gif":
		return gif.Decode(reader)
	}
	return nil, ErrPreviewType
}

func encodeThumbnail(picture *image.RGBA) (Thumbnail, error) {
	var encoded bytes.Buffer
	if picture.Opaque() {
		if err := jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
			return Thumbnail{}, err
		}
		return Thumbnail{ContentType: "image/jpeg", Contents: encoded.Bytes()}, nil
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&encoded, picture); err != nil {
		return Thumbnail{}, err
	}
	return Thumbnail{ContentType: "image/png", Contents: encoded.Bytes()}, nil
}

// thumbnailSize fits a picture inside thumbnailLongestEdge. A smaller picture keeps its size.
func thumbnailSize(width, height int) (int, int) {
	longest := max(width, height)
	if longest <= thumbnailLongestEdge {
		return width, height
	}
	return max(1, width*thumbnailLongestEdge/longest), max(1, height*thumbnailLongestEdge/longest)
}

// uprightPoint is the pixel of the stored picture that shows at (x, y) of the upright one, by the Exif orientation.
// width and height are those of the stored picture.
func uprightPoint(orientation, x, y, width, height int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return y, height - 1 - x
	case 7:
		return width - 1 - y, height - 1 - x
	case 8:
		return width - 1 - y, x
	}
	return x, y
}

// shrunkAndUpright averages a fixed number of points of the source for each pixel of the thumbnail.
func shrunkAndUpright(source image.Image, orientation int) *image.RGBA {
	bounds := source.Bounds()
	storedWidth, storedHeight := bounds.Dx(), bounds.Dy()
	uprightWidth, uprightHeight := storedWidth, storedHeight
	// Orientations 5 to 8 turn the picture on its side.
	if orientation >= 5 {
		uprightWidth, uprightHeight = storedHeight, storedWidth
	}
	width, height := thumbnailSize(uprightWidth, uprightHeight)
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	const samples = thumbnailSamplesPerAxis
	const sampleCount = samples * samples
	for y := range height {
		for x := range width {
			var red, green, blue, alpha uint32
			for sampleY := range samples {
				for sampleX := range samples {
					// The middle of each of the samples-by-samples parts of the thumbnail pixel.
					uprightX := (2*x*samples + 2*sampleX + 1) * uprightWidth / (2 * samples * width)
					uprightY := (2*y*samples + 2*sampleY + 1) * uprightHeight / (2 * samples * height)
					storedX, storedY := uprightPoint(orientation, uprightX, uprightY, storedWidth, storedHeight)
					r, g, b, a := source.At(bounds.Min.X+storedX, bounds.Min.Y+storedY).RGBA()
					red, green, blue, alpha = red+r, green+g, blue+b, alpha+a
				}
			}
			result.SetRGBA(x, y, color.RGBA{
				R: uint8(red / sampleCount >> 8),
				G: uint8(green / sampleCount >> 8),
				B: uint8(blue / sampleCount >> 8),
				A: uint8(alpha / sampleCount >> 8),
			})
		}
	}
	return result
}
