package images

import (
    "bytes"
    "encoding/binary"
    "hash/crc32"
    "testing"
)

// PNG DecodeConfig only reads IHDR, so bounds can be tested without allocating
// 24 million pixels.
func headerOnlyPNG(width, height uint32) []byte {
    var buf bytes.Buffer
    buf.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
    _ = binary.Write(&buf, binary.BigEndian, uint32(13))
    buf.WriteString("IHDR")
    ihdr := make([]byte, 13)
    binary.BigEndian.PutUint32(ihdr[:4], width)
    binary.BigEndian.PutUint32(ihdr[4:8], height)
    ihdr[8], ihdr[9] = 8, 2
    buf.Write(ihdr)
    _ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
    return buf.Bytes()
}

func TestUploadImageDimensions(t *testing.T) {
    for _, d := range [][2]uint32{{16000, 1}, {800, 12000}, {6000, 4000}} {
        if _, _, err := ValidateDirectImage(headerOnlyPNG(d[0], d[1]), "image/png"); err != nil {
            t.Fatalf("valid image %dx%d rejected: %v", d[0], d[1], err)
        }
    }
    for _, d := range [][2]uint32{{16001, 1}, {6001, 4000}, {800, 16001}} {
        if _, _, err := ValidateDirectImage(headerOnlyPNG(d[0], d[1]), "image/png"); err == nil {
            t.Fatalf("oversized image %dx%d accepted", d[0], d[1])
        }
    }
}
