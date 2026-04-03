// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package basecodec

// Encode encodes data to a string using the current active encoding scheme (default: LowerBase32)
func Encode(data []byte) string {
	// To switch to Base36, change this to: return EncodeLowerBase36(data)
	return EncodeLowerBase32(data)
}

// Decode decodes data from a byte slice using the current active encoding scheme
func Decode(data []byte) ([]byte, error) {
	// To switch to Base36, change this to: return DecodeLowerBase36(data)
	return DecodeLowerBase32(data)
}

// DecodeString decodes data from a string using the current active encoding scheme
func DecodeString(data string) ([]byte, error) {
	// To switch to Base36, change this to: return DecodeLowerBase36String(data)
	return DecodeLowerBase32String(data)
}
