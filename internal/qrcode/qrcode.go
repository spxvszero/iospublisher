package qrcode

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

const (
	version             = 10
	size                = 17 + 4*version
	dataCodewords       = 274
	ecCodewordsPerBlock = 18
	maxBytes            = 271
)

var blockDataLengths = []int{68, 68, 69, 69}
var alignmentPositions = []int{6, 28, 50}

func PNG(text string, scale, border int) ([]byte, error) {
	matrix, err := Matrix([]byte(text))
	if err != nil {
		return nil, err
	}
	if scale <= 0 {
		scale = 6
	}
	if border < 0 {
		border = 4
	}

	px := (len(matrix) + border*2) * scale
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	black := image.NewUniform(color.Black)
	for y := range matrix {
		for x := range matrix[y] {
			if !matrix[y][x] {
				continue
			}
			r := image.Rect((x+border)*scale, (y+border)*scale, (x+border+1)*scale, (y+border+1)*scale)
			draw.Draw(img, r, black, image.Point{}, draw.Src)
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Matrix(data []byte) ([][]bool, error) {
	if len(data) == 0 {
		return nil, errors.New("qr content is required")
	}
	if len(data) > maxBytes {
		return nil, errors.New("qr content is too long")
	}

	q := newQR()
	q.drawFunctionPatterns()
	q.addData(interleaveBlocks(encodeData(data)))
	q.applyMask(0)
	q.drawFormatBits(0)
	return q.modules, nil
}

type qr struct {
	modules  [][]bool
	function [][]bool
}

func newQR() *qr {
	modules := make([][]bool, size)
	function := make([][]bool, size)
	for i := 0; i < size; i++ {
		modules[i] = make([]bool, size)
		function[i] = make([]bool, size)
	}
	return &qr{modules: modules, function: function}
}

func (q *qr) drawFunctionPatterns() {
	q.drawFinderPattern(3, 3)
	q.drawFinderPattern(size-4, 3)
	q.drawFinderPattern(3, size-4)

	for i := 0; i < size; i++ {
		if !q.function[6][i] {
			q.setFunction(i, 6, i%2 == 0)
		}
		if !q.function[i][6] {
			q.setFunction(6, i, i%2 == 0)
		}
	}

	for i, x := range alignmentPositions {
		for j, y := range alignmentPositions {
			if (i == 0 && j == 0) || (i == len(alignmentPositions)-1 && j == 0) || (i == 0 && j == len(alignmentPositions)-1) {
				continue
			}
			q.drawAlignmentPattern(x, y)
		}
	}

	q.setFunction(8, size-8, true)
	q.drawFormatBits(0)
	q.drawVersion()
}

func (q *qr) drawFinderPattern(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || x >= size || y < 0 || y >= size {
				continue
			}
			dist := max(abs(dx), abs(dy))
			q.setFunction(x, y, dist == 3 || dist <= 1)
		}
	}
}

func (q *qr) drawAlignmentPattern(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			dist := max(abs(dx), abs(dy))
			q.setFunction(cx+dx, cy+dy, dist == 2 || dist == 0)
		}
	}
}

func (q *qr) drawFormatBits(mask int) {
	const eclLow = 1
	data := eclLow<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := ((data << 10) | (rem & 0x3FF)) ^ 0x5412

	for i := 0; i <= 5; i++ {
		q.setFunction(8, i, bit(bits, i))
	}
	q.setFunction(8, 7, bit(bits, 6))
	q.setFunction(8, 8, bit(bits, 7))
	q.setFunction(7, 8, bit(bits, 8))
	for i := 9; i < 15; i++ {
		q.setFunction(14-i, 8, bit(bits, i))
	}
	for i := 0; i < 8; i++ {
		q.setFunction(size-1-i, 8, bit(bits, i))
	}
	for i := 8; i < 15; i++ {
		q.setFunction(8, size-15+i, bit(bits, i))
	}
	q.setFunction(8, size-8, true)
}

func (q *qr) drawVersion() {
	rem := version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	bits := (version << 12) | (rem & 0xFFF)
	for i := 0; i < 18; i++ {
		x := size - 11 + i%3
		y := i / 3
		dark := bit(bits, i)
		q.setFunction(x, y, dark)
		q.setFunction(y, x, dark)
	}
}

func (q *qr) addData(codewords []byte) {
	bitLen := len(codewords) * 8
	bitIndex := 0
	upward := true
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right--
		}
		for vert := 0; vert < size; vert++ {
			var y int
			if upward {
				y = size - 1 - vert
			} else {
				y = vert
			}
			for dx := 0; dx < 2; dx++ {
				x := right - dx
				if q.function[y][x] {
					continue
				}
				dark := false
				if bitIndex < bitLen {
					dark = ((codewords[bitIndex>>3] >> uint(7-(bitIndex&7))) & 1) != 0
					bitIndex++
				}
				q.modules[y][x] = dark
			}
		}
		upward = !upward
	}
}

func (q *qr) applyMask(mask int) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !q.function[y][x] && maskBit(mask, x, y) {
				q.modules[y][x] = !q.modules[y][x]
			}
		}
	}
}

func (q *qr) setFunction(x, y int, dark bool) {
	q.modules[y][x] = dark
	q.function[y][x] = true
}

func encodeData(data []byte) []byte {
	bits := newBitBuffer()
	bits.append(0b0100, 4)
	bits.append(uint(len(data)), 16)
	for _, b := range data {
		bits.append(uint(b), 8)
	}
	bits.append(0, min(4, dataCodewords*8-bits.len()))
	for bits.len()%8 != 0 {
		bits.append(0, 1)
	}

	out := bits.bytes()
	for pad := byte(0xEC); len(out) < dataCodewords; {
		out = append(out, pad)
		if pad == 0xEC {
			pad = 0x11
		} else {
			pad = 0xEC
		}
	}
	return out
}

func interleaveBlocks(data []byte) []byte {
	blocks := make([][]byte, len(blockDataLengths))
	eccBlocks := make([][]byte, len(blockDataLengths))
	offset := 0
	for i, length := range blockDataLengths {
		blocks[i] = data[offset : offset+length]
		eccBlocks[i] = reedSolomonRemainder(blocks[i], ecCodewordsPerBlock)
		offset += length
	}

	var out []byte
	for i := 0; i < 69; i++ {
		for _, block := range blocks {
			if i < len(block) {
				out = append(out, block[i])
			}
		}
	}
	for i := 0; i < ecCodewordsPerBlock; i++ {
		for _, block := range eccBlocks {
			out = append(out, block[i])
		}
	}
	return out
}

type bitBuffer struct {
	data []byte
	n    int
}

func newBitBuffer() *bitBuffer {
	return &bitBuffer{}
}

func (b *bitBuffer) append(value uint, width int) {
	for i := width - 1; i >= 0; i-- {
		if b.n%8 == 0 {
			b.data = append(b.data, 0)
		}
		if ((value >> uint(i)) & 1) != 0 {
			b.data[len(b.data)-1] |= 1 << uint(7-(b.n%8))
		}
		b.n++
	}
}

func (b *bitBuffer) len() int {
	return b.n
}

func (b *bitBuffer) bytes() []byte {
	return append([]byte(nil), b.data...)
}

func reedSolomonRemainder(data []byte, degree int) []byte {
	gen := reedSolomonGenerator(degree)
	work := make([]byte, len(data)+degree)
	copy(work, data)
	for i := range data {
		coef := work[i]
		if coef == 0 {
			continue
		}
		for j := 1; j < len(gen); j++ {
			work[i+j] ^= gfMul(gen[j], coef)
		}
	}
	return work[len(data):]
}

func reedSolomonGenerator(degree int) []byte {
	gen := []byte{1}
	root := byte(1)
	for i := 0; i < degree; i++ {
		next := make([]byte, len(gen)+1)
		for j, coef := range gen {
			next[j] ^= coef
			next[j+1] ^= gfMul(coef, root)
		}
		gen = next
		root = gfMul(root, 2)
	}
	return gen
}

func gfMul(x, y byte) byte {
	z := 0
	a := int(x)
	b := int(y)
	for b != 0 {
		if b&1 != 0 {
			z ^= a
		}
		a <<= 1
		if a&0x100 != 0 {
			a ^= 0x11D
		}
		b >>= 1
	}
	return byte(z)
}

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	case 7:
		return ((x+y)%2+(x*y)%3)%2 == 0
	default:
		return false
	}
}

func bit(value, index int) bool {
	return ((value >> index) & 1) != 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
