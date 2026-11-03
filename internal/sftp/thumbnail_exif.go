package sftp

import (
	"bytes"
	"encoding/binary"
)

const (
	jpegMarkerStartOfScan              = 0xDA
	jpegMarkerEndOfImage               = 0xD9
	jpegMarkerApp1                     = 0xE1
	jpegMarkerProgressiveFrame         = 0xC2
	exifTagOrientation                 = 0x0112
	exifTagThumbnailOffset             = 0x0201
	exifTagThumbnailLength             = 0x0202
	exifTypeShort                      = 3
	exifEntryBytes                     = 12
	exifUprightOrientation             = 1
	exifLastOrientation                = 8
	exifHeaderBytes                    = len("Exif\x00\x00")
	tiffHeaderBytes                    = 8
	jpegSegmentHeaderBytes             = 4
	jpegSegmentLengthFieldBytes        = 2
	jpegStartOfImageBytes              = 2
	exifValueOffsetInEntry             = 8
	exifIFDEntryCountBytes             = 2
	exifNextIFDOffsetBytes             = 4
	tiffFirstIFDOffsetPosition         = 4
	tiffMagicPosition                  = 2
	tiffMagicNumber             uint16 = 42
)

// jpegExif is what a thumbnail needs from a JPEG's Exif header.
type jpegExif struct {
	// orientation is how to turn the pixels to see the picture upright (1 to 8, as Exif numbers them). The browser
	// applies it to a picture it shows whole, so a thumbnail that ignored it would lie on its side.
	orientation int
	// embedded is the small JPEG a camera put in the header, or nil.
	embedded []byte
}

// readJPEGExif reads the Exif header in the leading bytes of a JPEG. The file comes from the host and is not trusted:
// every offset is checked against the bytes at hand, and anything that does not add up is treated as absent.
func readJPEGExif(head []byte) jpegExif {
	exif := jpegExif{orientation: exifUprightOrientation}
	eachJPEGSegment(head, func(marker byte, segment []byte) bool {
		if marker != jpegMarkerApp1 || !bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return true
		}
		readTIFFHeader(segment[exifHeaderBytes:], &exif)
		return false
	})
	return exif
}

// jpegIsProgressive says whether the frame header in data, which a successful DecodeConfig has seen, is progressive.
func jpegIsProgressive(data []byte) bool {
	progressive := false
	eachJPEGSegment(data, func(marker byte, _ []byte) bool {
		progressive = marker == jpegMarkerProgressiveFrame
		return !progressive
	})
	return progressive
}

// eachJPEGSegment walks the marker segments before the image data, until visit returns false or the bytes run out.
func eachJPEGSegment(data []byte, visit func(marker byte, segment []byte) bool) {
	position := jpegStartOfImageBytes
	for position+jpegSegmentHeaderBytes <= len(data) {
		if data[position] != 0xFF {
			return
		}
		marker := data[position+1]
		// A marker may be padded with 0xFF bytes.
		if marker == 0xFF {
			position++
			continue
		}
		if marker == jpegMarkerStartOfScan || marker == jpegMarkerEndOfImage {
			return
		}
		length := int(binary.BigEndian.Uint16(data[position+2:]))
		if length < jpegSegmentLengthFieldBytes {
			return
		}
		end := position + jpegSegmentLengthFieldBytes + length
		if end > len(data) {
			return
		}
		if !visit(marker, data[position+jpegSegmentHeaderBytes:end]) {
			return
		}
		position = end
	}
}

// readTIFFHeader fills exif from the TIFF structure inside an Exif segment. Only two directories are read, so a file
// whose offsets point in a circle cannot keep it walking.
func readTIFFHeader(tiff []byte, exif *jpegExif) {
	if len(tiff) < tiffHeaderBytes {
		return
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(tiff[tiffMagicPosition:]) != tiffMagicNumber {
		return
	}
	first := readIFD(tiff, order, order.Uint32(tiff[tiffFirstIFDOffsetPosition:]))
	if orientation := first.orientation; orientation >= exifUprightOrientation && orientation <= exifLastOrientation {
		exif.orientation = orientation
	}
	second := readIFD(tiff, order, first.next)
	exif.embedded = embeddedJPEG(tiff, second)
}

// exifDirectory is the tags of one TIFF directory that a thumbnail reads.
type exifDirectory struct {
	orientation     int
	thumbnailOffset uint32
	thumbnailLength uint32
	next            uint32
}

func readIFD(tiff []byte, order binary.ByteOrder, offset uint32) exifDirectory {
	var directory exifDirectory
	if offset == 0 || uint64(offset)+exifIFDEntryCountBytes > uint64(len(tiff)) {
		return directory
	}
	start := int(offset) + exifIFDEntryCountBytes
	count := int(order.Uint16(tiff[offset:]))
	if start+count*exifEntryBytes > len(tiff) {
		return directory
	}
	for index := range count {
		entry := tiff[start+index*exifEntryBytes:]
		switch order.Uint16(entry) {
		case exifTagOrientation:
			if order.Uint16(entry[2:]) == exifTypeShort {
				directory.orientation = int(order.Uint16(entry[exifValueOffsetInEntry:]))
			}
		case exifTagThumbnailOffset:
			directory.thumbnailOffset = order.Uint32(entry[exifValueOffsetInEntry:])
		case exifTagThumbnailLength:
			directory.thumbnailLength = order.Uint32(entry[exifValueOffsetInEntry:])
		}
	}
	nextPosition := start + count*exifEntryBytes
	if nextPosition+exifNextIFDOffsetBytes <= len(tiff) {
		directory.next = order.Uint32(tiff[nextPosition:])
	}
	return directory
}

func embeddedJPEG(tiff []byte, directory exifDirectory) []byte {
	end := uint64(directory.thumbnailOffset) + uint64(directory.thumbnailLength)
	if directory.thumbnailLength == 0 || end > uint64(len(tiff)) {
		return nil
	}
	embedded := tiff[directory.thumbnailOffset:end]
	if len(embedded) < jpegStartOfImageBytes || embedded[0] != 0xFF || embedded[1] != 0xD8 {
		return nil
	}
	return embedded
}
