package sftp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

// countingReader counts how much of a file is read, which over SFTP is what a thumbnail costs.
type countingReader struct {
	reader io.Reader
	read   int
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	r.read += read
	return read, err
}

func filledPicture(width, height int, fill color.Color) *image.RGBA {
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, fill)
		}
	}
	return picture
}

func encodedJPEG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func encodedPNG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// exifDocument builds the TIFF structure of an Exif segment: the orientation in the first directory and, when embedded
// is given, that JPEG in the second.
func exifDocument(orientation uint16, embedded []byte) []byte {
	var tiff bytes.Buffer
	order := binary.LittleEndian
	tiff.WriteString("II")
	_ = binary.Write(&tiff, order, tiffMagicNumber)
	_ = binary.Write(&tiff, order, uint32(tiffHeaderBytes))
	const firstEntries, secondEntries = 1, 2
	secondOffset := tiffHeaderBytes + exifIFDEntryCountBytes + firstEntries*exifEntryBytes + exifNextIFDOffsetBytes
	embeddedOffset := secondOffset + exifIFDEntryCountBytes + secondEntries*exifEntryBytes + exifNextIFDOffsetBytes
	_ = binary.Write(&tiff, order, uint16(firstEntries))
	writeExifEntry(&tiff, exifTagOrientation, exifTypeShort, uint32(orientation))
	if embedded == nil {
		_ = binary.Write(&tiff, order, uint32(0))
		return tiff.Bytes()
	}
	_ = binary.Write(&tiff, order, uint32(secondOffset))
	_ = binary.Write(&tiff, order, uint16(secondEntries))
	writeExifEntry(&tiff, exifTagThumbnailOffset, 4, uint32(embeddedOffset))
	writeExifEntry(&tiff, exifTagThumbnailLength, 4, uint32(len(embedded)))
	_ = binary.Write(&tiff, order, uint32(0))
	tiff.Write(embedded)
	return tiff.Bytes()
}

func writeExifEntry(tiff *bytes.Buffer, tag, valueType uint16, value uint32) {
	order := binary.LittleEndian
	_ = binary.Write(tiff, order, tag)
	_ = binary.Write(tiff, order, valueType)
	_ = binary.Write(tiff, order, uint32(1))
	_ = binary.Write(tiff, order, value)
}

// jpegWithExif puts an Exif segment right after the start of a JPEG, where cameras put it.
func jpegWithExif(t *testing.T, plain []byte, tiff []byte) []byte {
	t.Helper()
	segment := append([]byte("Exif\x00\x00"), tiff...)
	if len(segment)+jpegSegmentLengthFieldBytes > 0xFFFF {
		t.Fatalf("Exif segment of %d bytes does not fit", len(segment))
	}
	var result bytes.Buffer
	result.Write(plain[:jpegStartOfImageBytes])
	result.Write([]byte{0xFF, jpegMarkerApp1})
	_ = binary.Write(&result, binary.BigEndian, uint16(len(segment)+jpegSegmentLengthFieldBytes))
	result.Write(segment)
	result.Write(plain[jpegStartOfImageBytes:])
	return result.Bytes()
}

func madeThumbnail(t *testing.T, contents []byte) Thumbnail {
	t.Helper()
	source, err := readThumbnailSource(context.Background(), bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	thumbnail, err := renderThumbnail(source)
	if err != nil {
		t.Fatal(err)
	}
	return thumbnail
}

func decodedThumbnail(t *testing.T, thumbnail Thumbnail) image.Image {
	t.Helper()
	picture, _, err := image.Decode(bytes.NewReader(thumbnail.Contents))
	if err != nil {
		t.Fatal(err)
	}
	return picture
}

func TestALargePictureShrinksToTheLongestEdgeAndKeepsItsShape(t *testing.T) {
	contents := encodedJPEG(t, filledPicture(1000, 500, color.RGBA{R: 200, A: 255}))
	thumbnail := madeThumbnail(t, contents)
	if thumbnail.ContentType != "image/jpeg" {
		t.Fatalf("type = %q", thumbnail.ContentType)
	}
	if size := decodedThumbnail(t, thumbnail).Bounds().Size(); size != image.Pt(thumbnailLongestEdge, thumbnailLongestEdge/2) {
		t.Fatalf("size = %v", size)
	}
}

func TestASmallPictureKeepsItsSize(t *testing.T) {
	thumbnail := madeThumbnail(t, encodedPNG(t, filledPicture(40, 30, color.RGBA{G: 200, A: 255})))
	if size := decodedThumbnail(t, thumbnail).Bounds().Size(); size != image.Pt(40, 30) {
		t.Fatalf("size = %v", size)
	}
}

func TestOnlyAPictureWithTransparentPixelsStaysPNG(t *testing.T) {
	opaque := madeThumbnail(t, encodedPNG(t, filledPicture(20, 20, color.RGBA{B: 200, A: 255})))
	if opaque.ContentType != "image/jpeg" {
		t.Fatalf("opaque type = %q", opaque.ContentType)
	}
	clear := madeThumbnail(t, encodedPNG(t, filledPicture(20, 20, color.RGBA{})))
	if clear.ContentType != "image/png" {
		t.Fatalf("transparent type = %q", clear.ContentType)
	}
}

func TestAGIFShowsItsFirstFrame(t *testing.T) {
	palette := color.Palette{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	red, blue := image.NewPaletted(image.Rect(0, 0, 8, 8), palette), image.NewPaletted(image.Rect(0, 0, 8, 8), palette)
	for index := range blue.Pix {
		blue.Pix[index] = 1
	}
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{red, blue}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := decodedThumbnail(t, madeThumbnail(t, encoded.Bytes())).At(4, 4).RGBA()
	if r < b {
		t.Fatalf("the first frame is red, got r=%d b=%d", r, b)
	}
}

func TestAFileThatIsNotADecodablePictureIsRefused(t *testing.T) {
	tests := map[string][]byte{
		"svg, whatever it is named":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"text":                        []byte("hello"),
		"empty":                       {},
		"png signature and no more":   []byte("\x89PNG\r\n\x1a\n"),
		"jpeg start and no more":      {0xFF, 0xD8, 0xFF, 0xE0},
		"webp the engine cannot read": []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			source, err := readThumbnailSource(context.Background(), bytes.NewReader(contents), int64(len(contents)))
			if err == nil {
				_, err = renderThumbnail(source)
			}
			if !errors.Is(err, ErrPreviewType) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// pngDeclaring is the start of a PNG whose header promises a size; the pixels never come.
func pngDeclaring(width, height uint32) []byte {
	var contents bytes.Buffer
	contents.WriteString("\x89PNG\r\n\x1a\n")
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, width)
	binary.BigEndian.PutUint32(header[4:], height)
	header[8], header[9] = 8, 6 // 8 bits per channel, RGBA.
	_ = binary.Write(&contents, binary.BigEndian, uint32(len(header)))
	chunk := append([]byte("IHDR"), header...)
	contents.Write(chunk)
	_ = binary.Write(&contents, binary.BigEndian, crc32PNG(chunk))
	return contents.Bytes()
}

func TestADecompressionBombIsRefusedFromItsHeaderBeforeTheRestIsRead(t *testing.T) {
	bomb := append(pngDeclaring(50_000, 50_000), make([]byte, 1<<20)...)
	counted := &countingReader{reader: bytes.NewReader(bomb)}
	_, err := readThumbnailSource(context.Background(), counted, int64(len(bomb)))
	if !errors.Is(err, ErrPreviewTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if counted.read > previewSniffBytes {
		t.Fatalf("read %d bytes of a file the first %d refuse", counted.read, previewSniffBytes)
	}
}

func TestAProgressiveJPEGCountsTheCoefficientsItsDecoderHolds(t *testing.T) {
	baseline := jpegHeaderDeclaring(0xC0, 4000, 3000)
	progressive := jpegHeaderDeclaring(0xC2, 4000, 3000)
	for name, test := range map[string]struct {
		header []byte
		want   int64
	}{
		"baseline":    {baseline, 4000 * 3000 * 3},
		"progressive": {progressive, 4000 * 3000 * 3 * progressiveDecodeFactor},
	} {
		t.Run(name, func(t *testing.T) {
			config, err := jpeg.DecodeConfig(bytes.NewReader(test.header))
			if err != nil {
				t.Fatal(err)
			}
			if got := decodedBytes("image/jpeg", test.header, config); got != test.want {
				t.Fatalf("decodedBytes = %d, want %d", got, test.want)
			}
		})
	}
}

// jpegHeaderDeclaring is a JPEG's start, a frame header with three components and the start of a scan, which is where
// DecodeConfig stops reading a file without a JFIF segment.
func jpegHeaderDeclaring(frameMarker byte, width, height uint16) []byte {
	var contents bytes.Buffer
	contents.Write([]byte{0xFF, 0xD8, 0xFF, frameMarker})
	_ = binary.Write(&contents, binary.BigEndian, uint16(17))
	contents.WriteByte(8)
	_ = binary.Write(&contents, binary.BigEndian, height)
	_ = binary.Write(&contents, binary.BigEndian, width)
	contents.Write([]byte{3, 1, 0x22, 0, 2, 0x11, 0, 3, 0x11, 0})
	contents.Write([]byte{0xFF, jpegMarkerStartOfScan, 0, 2})
	return contents.Bytes()
}

func TestABigFileWithoutAPictureInItsHeaderIsRefusedAfterItsFirstBytes(t *testing.T) {
	big := append(encodedPNG(t, filledPicture(10, 10, color.Black)), make([]byte, maxThumbnailSourceBytes)...)
	counted := &countingReader{reader: bytes.NewReader(big)}
	_, err := readThumbnailSource(context.Background(), counted, int64(len(big)))
	if !errors.Is(err, ErrPreviewTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if counted.read > previewSniffBytes {
		t.Fatalf("read %d bytes", counted.read)
	}
}

func TestACameraPhotoCostsOnlyItsHeader(t *testing.T) {
	embedded := encodedJPEG(t, filledPicture(160, 120, color.RGBA{R: 200, A: 255}))
	photo := jpegWithExif(t, encodedJPEG(t, filledPicture(16, 16, color.RGBA{B: 200, A: 255})), exifDocument(exifUprightOrientation, embedded))
	photo = append(photo, make([]byte, 3*maxThumbnailSourceBytes)...)
	counted := &countingReader{reader: bytes.NewReader(photo)}
	source, err := readThumbnailSource(context.Background(), counted, int64(len(photo)))
	if err != nil {
		t.Fatal(err)
	}
	if counted.read > thumbnailHeadBytes {
		t.Fatalf("read %d bytes of a photo with a picture in its header", counted.read)
	}
	thumbnail, err := renderThumbnail(source)
	if err != nil {
		t.Fatal(err)
	}
	picture := decodedThumbnail(t, thumbnail)
	if size := picture.Bounds().Size(); size != image.Pt(160, 120) {
		t.Fatalf("size = %v", size)
	}
	if r, _, b, _ := picture.At(80, 60).RGBA(); r < b {
		t.Fatalf("not the embedded picture: r=%d b=%d", r, b)
	}
}

func TestATinyPictureInTheHeaderIsNotWorthShowing(t *testing.T) {
	embedded := encodedJPEG(t, filledPicture(minEmbeddedThumbnailEdge-1, 40, color.RGBA{R: 200, A: 255}))
	photo := jpegWithExif(t, encodedJPEG(t, filledPicture(200, 200, color.RGBA{B: 200, A: 255})), exifDocument(exifUprightOrientation, embedded))
	if size := decodedThumbnail(t, madeThumbnail(t, photo)).Bounds().Size(); size != image.Pt(200, 200) {
		t.Fatalf("size = %v, want the file itself", size)
	}
}

func TestAPhotoHeldSidewaysComesOutUpright(t *testing.T) {
	// Stored wide, red on the left and blue on the right. Orientation 6 means: turn it clockwise to see it, so the left
	// edge becomes the top.
	wide := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := range 100 {
		for x := range 200 {
			if x < 100 {
				wide.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				wide.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	photo := jpegWithExif(t, encodedJPEG(t, wide), exifDocument(6, nil))
	picture := decodedThumbnail(t, madeThumbnail(t, photo))
	if size := picture.Bounds().Size(); size != image.Pt(100, 200) {
		t.Fatalf("size = %v", size)
	}
	if r, _, b, _ := picture.At(50, 20).RGBA(); r < b {
		t.Fatalf("the top should be red: r=%d b=%d", r, b)
	}
	if r, _, b, _ := picture.At(50, 180).RGBA(); r > b {
		t.Fatalf("the bottom should be blue: r=%d b=%d", r, b)
	}
}

func TestEveryOrientationShowsTheSameUprightPicture(t *testing.T) {
	// The Exif specification's definition: orientation N says where the stored picture's first row and first column
	// appear when it is upright. Orientation 6, for one: the first row is the right side, read from the top.
	upright := [][]int{{1, 2}, {3, 4}, {5, 6}}
	const uprightWidth, uprightHeight = 2, 3
	stored := map[int][][]int{
		1: {{1, 2}, {3, 4}, {5, 6}},
		2: {{2, 1}, {4, 3}, {6, 5}},
		3: {{6, 5}, {4, 3}, {2, 1}},
		4: {{5, 6}, {3, 4}, {1, 2}},
		5: {{1, 3, 5}, {2, 4, 6}},
		6: {{2, 4, 6}, {1, 3, 5}},
		7: {{6, 4, 2}, {5, 3, 1}},
		8: {{5, 3, 1}, {6, 4, 2}},
	}
	for orientation, rows := range stored {
		height, width := len(rows), len(rows[0])
		for y := range uprightHeight {
			for x := range uprightWidth {
				storedX, storedY := uprightPoint(orientation, x, y, width, height)
				if got := rows[storedY][storedX]; got != upright[y][x] {
					t.Errorf("orientation %d at (%d,%d) shows %d, want %d", orientation, x, y, got, upright[y][x])
				}
			}
		}
	}
}

func TestAnExifHeaderThatDoesNotAddUpIsIgnored(t *testing.T) {
	plain := encodedJPEG(t, filledPicture(16, 16, color.Black))
	huge := exifDocument(6, nil)
	binary.LittleEndian.PutUint32(huge[tiffFirstIFDOffsetPosition:], 0xFFFFFFF0)
	tests := map[string]struct {
		tiff        []byte
		orientation int
	}{
		"not tiff":                 {[]byte("not a tiff header"), exifUprightOrientation},
		"directory past the end":   {huge, exifUprightOrientation},
		"orientation out of range": {exifDocument(99, nil), exifUprightOrientation},
		"empty":                    {nil, exifUprightOrientation},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			exif := readJPEGExif(jpegWithExif(t, plain, test.tiff))
			if exif.orientation != test.orientation || exif.embedded != nil {
				t.Fatalf("exif = %+v", exif)
			}
		})
	}
	t.Run("embedded picture past the end", func(t *testing.T) {
		tiff := exifDocument(1, encodedJPEG(t, filledPicture(160, 120, color.Black)))
		exif := readJPEGExif(jpegWithExif(t, plain, tiff[:len(tiff)-100]))
		if exif.embedded != nil {
			t.Fatal("kept a picture that runs past the segment")
		}
	})
}

func TestACancelledRequestStopsReadingBetweenChunks(t *testing.T) {
	big := make([]byte, maxThumbnailSourceBytes)
	copy(big, encodedPNG(t, filledPicture(10, 10, color.Black)))
	ctx, cancel := context.WithCancel(context.Background())
	counted := &countingReader{reader: bytes.NewReader(big)}
	cancelling := readerThatCancels{reader: counted, after: thumbnailReadChunkBytes, cancel: cancel, counted: counted}
	_, err := readThumbnailSource(ctx, cancelling, int64(len(big)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if counted.read >= len(big) {
		t.Fatalf("read the whole file after the cancellation: %d", counted.read)
	}
}

type readerThatCancels struct {
	reader  io.Reader
	after   int
	cancel  func()
	counted *countingReader
}

func (r readerThatCancels) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	if r.counted.read >= r.after {
		r.cancel()
	}
	return read, err
}

func FuzzThumbnailOfAnyBytes(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add(jpegHeaderDeclaring(0xC2, 100, 100))
	f.Fuzz(func(t *testing.T, contents []byte) {
		readJPEGExif(contents)
		source, err := readThumbnailSource(context.Background(), bytes.NewReader(contents), int64(len(contents)))
		if err == nil {
			_, _ = renderThumbnail(source)
		}
	})
}

func crc32PNG(chunk []byte) uint32 { return crc32.ChecksumIEEE(chunk) }
