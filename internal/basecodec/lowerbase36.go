// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package basecodec

import (
	"errors"
	"math/bits"
)

var (
	ErrInvalidLowerBase36 = errors.New("invalid lower base36 data")

	lowerBase36Alphabet = [36]byte{
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j',
		'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't',
		'u', 'v', 'w', 'x', 'y', 'z',
	}
	lowerBase36DecodeMap = newLowerBase36DecodeMap()
)

func EncodeLowerBase36(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	leadingZeros := 0
	for leadingZeros < len(data) && data[leadingZeros] == 0 {
		leadingZeros++
	}

	if leadingZeros == len(data) {
		encoded := make([]byte, leadingZeros)
		for i := range encoded {
			encoded[i] = '0'
		}
		return string(encoded)
	}

	significant := data[leadingZeros:]
	if len(significant) <= 8 {
		return encodeLowerBase36Small(significant, leadingZeros)
	}

	tmp := make([]byte, len(significant))
	copy(tmp, significant)

	encoded := make([]byte, 0, len(tmp)*2+leadingZeros)
	for start := 0; start < len(tmp); {
		remainder := 0
		for i := start; i < len(tmp); i++ {
			value := remainder*256 + int(tmp[i])
			tmp[i] = byte(value / 36)
			remainder = value % 36
		}
		encoded = append(encoded, lowerBase36Alphabet[remainder])
		for start < len(tmp) && tmp[start] == 0 {
			start++
		}
	}

	for i := 0; i < leadingZeros; i++ {
		encoded = append(encoded, '0')
	}

	reverseBytes(encoded)
	return string(encoded)
}

func DecodeLowerBase36(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	leadingZeros := 0
	for leadingZeros < len(data) && data[leadingZeros] == '0' {
		leadingZeros++
	}

	if leadingZeros == len(data) {
		return make([]byte, leadingZeros), nil
	}

	significant := data[leadingZeros:]
	if len(significant) <= 12 {
		return decodeLowerBase36Small(significant, leadingZeros)
	}

	decodedLE := make([]byte, 0, len(significant)*13/20+1)
	for _, ch := range significant {
		value := lowerBase36DecodeMap[ch]
		if value == 0xFF {
			return nil, ErrInvalidLowerBase36
		}

		carry := int(value)
		for i := 0; i < len(decodedLE); i++ {
			carry += int(decodedLE[i]) * 36
			decodedLE[i] = byte(carry)
			carry >>= 8
		}
		for carry > 0 {
			decodedLE = append(decodedLE, byte(carry))
			carry >>= 8
		}
	}

	decoded := make([]byte, leadingZeros+len(decodedLE))
	for i := 0; i < len(decodedLE); i++ {
		decoded[len(decoded)-1-i] = decodedLE[i]
	}

	return decoded, nil
}

func encodeLowerBase36Small(data []byte, leadingZeros int) string {
	var value uint64
	for i := 0; i < len(data); i++ {
		value = (value << 8) | uint64(data[i])
	}

	var reversed [13]byte
	n := 0
	for value > 0 {
		reversed[n] = lowerBase36Alphabet[value%36]
		value /= 36
		n++
	}
	if n == 0 {
		reversed[n] = '0'
		n++
	}

	encoded := make([]byte, leadingZeros+n)
	for i := 0; i < leadingZeros; i++ {
		encoded[i] = '0'
	}
	for i := 0; i < n; i++ {
		encoded[leadingZeros+i] = reversed[n-1-i]
	}
	return string(encoded)
}

func decodeLowerBase36Small(data []byte, leadingZeros int) ([]byte, error) {
	var value uint64
	for _, ch := range data {
		digit := lowerBase36DecodeMap[ch]
		if digit == 0xFF {
			return nil, ErrInvalidLowerBase36
		}
		value = value*36 + uint64(digit)
	}

	byteLen := (bits.Len64(value) + 7) / 8
	if byteLen == 0 {
		byteLen = 1
	}

	decoded := make([]byte, leadingZeros+byteLen)
	for i := 0; i < byteLen; i++ {
		decoded[len(decoded)-1-i] = byte(value)
		value >>= 8
	}
	return decoded, nil
}

func newLowerBase36DecodeMap() [256]byte {
	var table [256]byte
	for i := range table {
		table[i] = 0xFF
	}
	for i, ch := range lowerBase36Alphabet {
		table[ch] = byte(i)
	}
	return table
}

func reverseBytes(data []byte) {
	for left, right := 0, len(data)-1; left < right; left, right = left+1, right-1 {
		data[left], data[right] = data[right], data[left]
	}
}
