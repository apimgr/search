package geoip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync"
)

// MMDB file format constants
const (
	metadataStartMarker = "\xab\xcd\xefMaxMind.com"
)

// mmdbReader is a pure Go MMDB reader
type mmdbReader struct {
	mu       sync.RWMutex
	data     []byte
	metadata *mmdbMetadata
	nodeSize int
	dataOffset int
}

type mmdbMetadata struct {
	NodeCount        uint32
	RecordSize       uint16
	IPVersion        uint16
	DatabaseType     string
	Languages        []string
	BinaryFormatMajorVersion uint16
	BinaryFormatMinorVersion uint16
	BuildEpoch       uint64
	Description      map[string]string
}

// openMMDB opens an MMDB database file
func openMMDB(path string) (*mmdbReader, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	reader := &mmdbReader{
		data: data,
	}

	if err := reader.parseMetadata(); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return reader, nil
}

// parseMetadata parses the MMDB metadata from the end of the file
func (r *mmdbReader) parseMetadata() error {
	// Find metadata marker
	markerIdx := bytes.LastIndex(r.data, []byte(metadataStartMarker))
	if markerIdx == -1 {
		return errors.New("metadata marker not found")
	}

	metadataStart := markerIdx + len(metadataStartMarker)
	metadataBytes := r.data[metadataStart:]

	// Parse metadata map
	meta, _, err := r.decodeValue(metadataBytes, 0)
	if err != nil {
		return fmt.Errorf("failed to decode metadata: %w", err)
	}

	metaMap, ok := meta.(map[string]interface{})
	if !ok {
		return errors.New("metadata is not a map")
	}

	r.metadata = &mmdbMetadata{}

	if v, ok := metaMap["node_count"].(uint64); ok {
		r.metadata.NodeCount = uint32(v)
	}
	if v, ok := metaMap["record_size"].(uint64); ok {
		r.metadata.RecordSize = uint16(v)
	}
	if v, ok := metaMap["ip_version"].(uint64); ok {
		r.metadata.IPVersion = uint16(v)
	}
	if v, ok := metaMap["database_type"].(string); ok {
		r.metadata.DatabaseType = v
	}
	if v, ok := metaMap["build_epoch"].(uint64); ok {
		r.metadata.BuildEpoch = v
	}

	// Calculate node size and data offset
	r.nodeSize = int(r.metadata.RecordSize) * 2 / 8
	if r.metadata.RecordSize % 4 != 0 {
		r.nodeSize++
	}
	r.dataOffset = int(r.metadata.NodeCount) * r.nodeSize + 16

	return nil
}

// decodeValue decodes a value from MMDB format
func (r *mmdbReader) decodeValue(data []byte, offset int) (interface{}, int, error) {
	if offset >= len(data) {
		return nil, offset, errors.New("offset out of bounds")
	}

	ctrlByte := data[offset]
	offset++

	dataType := (ctrlByte >> 5) & 0x07
	size := int(ctrlByte & 0x1f)

	// Extended type
	if dataType == 0 {
		if offset >= len(data) {
			return nil, offset, errors.New("unexpected end of data")
		}
		dataType = data[offset] + 7
		offset++
	}

	// Get actual size
	if size == 29 {
		if offset >= len(data) {
			return nil, offset, errors.New("unexpected end of data")
		}
		size = 29 + int(data[offset])
		offset++
	} else if size == 30 {
		if offset+1 >= len(data) {
			return nil, offset, errors.New("unexpected end of data")
		}
		size = 285 + int(binary.BigEndian.Uint16(data[offset:offset+2]))
		offset += 2
	} else if size == 31 {
		if offset+2 >= len(data) {
			return nil, offset, errors.New("unexpected end of data")
		}
		size = 65821 + int(data[offset])<<16 + int(binary.BigEndian.Uint16(data[offset+1:offset+3]))
		offset += 3
	}

	switch dataType {
	case 1: // pointer
		pointerSize := ((int(ctrlByte) >> 3) & 0x03) + 1
		var pointer int
		switch pointerSize {
		case 1:
			pointer = (int(ctrlByte)&0x07)<<8 + int(data[offset])
			offset++
		case 2:
			pointer = 2048 + (int(ctrlByte)&0x07)<<16 + int(binary.BigEndian.Uint16(data[offset:offset+2]))
			offset += 2
		case 3:
			pointer = 526336 + (int(ctrlByte)&0x07)<<24 + int(data[offset])<<16 + int(binary.BigEndian.Uint16(data[offset+1:offset+3]))
			offset += 3
		case 4:
			pointer = int(binary.BigEndian.Uint32(data[offset : offset+4]))
			offset += 4
		}
		val, _, err := r.decodeValue(data, pointer)
		return val, offset, err

	case 2: // UTF-8 string
		if offset+size > len(data) {
			return "", offset, nil
		}
		return string(data[offset : offset+size]), offset + size, nil

	case 3: // double
		if offset+8 > len(data) {
			return 0.0, offset, nil
		}
		bits := binary.BigEndian.Uint64(data[offset : offset+8])
		return float64(bits), offset + 8, nil

	case 4: // bytes
		if offset+size > len(data) {
			return []byte{}, offset, nil
		}
		return data[offset : offset+size], offset + size, nil

	case 5: // uint16
		if size == 0 {
			return uint64(0), offset, nil
		}
		if offset+size > len(data) {
			return uint64(0), offset, nil
		}
		val := uint64(0)
		for i := 0; i < size; i++ {
			val = val<<8 | uint64(data[offset+i])
		}
		return val, offset + size, nil

	case 6: // uint32
		if size == 0 {
			return uint64(0), offset, nil
		}
		if offset+size > len(data) {
			return uint64(0), offset, nil
		}
		val := uint64(0)
		for i := 0; i < size; i++ {
			val = val<<8 | uint64(data[offset+i])
		}
		return val, offset + size, nil

	case 7: // map
		result := make(map[string]interface{})
		for i := 0; i < size; i++ {
			key, newOffset, err := r.decodeValue(data, offset)
			if err != nil {
				return result, newOffset, err
			}
			offset = newOffset

			val, newOffset, err := r.decodeValue(data, offset)
			if err != nil {
				return result, newOffset, err
			}
			offset = newOffset

			if keyStr, ok := key.(string); ok {
				result[keyStr] = val
			}
		}
		return result, offset, nil

	case 8: // int32
		if size == 0 {
			return int64(0), offset, nil
		}
		if offset+size > len(data) {
			return int64(0), offset, nil
		}
		val := int64(0)
		for i := 0; i < size; i++ {
			val = val<<8 | int64(data[offset+i])
		}
		// Sign extend if negative
		if size < 4 && val >= (1<<(uint(size)*8-1)) {
			val -= 1 << (uint(size) * 8)
		}
		return val, offset + size, nil

	case 9: // uint64
		if size == 0 {
			return uint64(0), offset, nil
		}
		if offset+size > len(data) {
			return uint64(0), offset, nil
		}
		val := uint64(0)
		for i := 0; i < size; i++ {
			val = val<<8 | uint64(data[offset+i])
		}
		return val, offset + size, nil

	case 10: // uint128
		if offset+size > len(data) {
			return big.NewInt(0), offset, nil
		}
		val := new(big.Int).SetBytes(data[offset : offset+size])
		return val, offset + size, nil

	case 11: // array
		result := make([]interface{}, 0, size)
		for i := 0; i < size; i++ {
			val, newOffset, err := r.decodeValue(data, offset)
			if err != nil {
				return result, newOffset, err
			}
			offset = newOffset
			result = append(result, val)
		}
		return result, offset, nil

	case 14: // bool
		return size != 0, offset, nil

	case 15: // float
		if offset+4 > len(data) {
			return float32(0), offset, nil
		}
		bits := binary.BigEndian.Uint32(data[offset : offset+4])
		return float32(bits), offset + 4, nil

	default:
		return nil, offset + size, nil
	}
}

// lookup finds the data offset for an IP address
func (r *mmdbReader) lookup(ip net.IP) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.metadata == nil {
		return 0, errors.New("database not loaded")
	}

	// Convert to 16-byte format
	ip16 := ip.To16()
	if ip16 == nil {
		return 0, errors.New("invalid IP address")
	}

	// For IPv4, start at bit 96 (first 96 bits are zero in mapped address)
	bitCount := 128
	startBit := 0
	if ip.To4() != nil && r.metadata.IPVersion == 4 {
		startBit = 96
	}

	nodeNum := uint32(0)
	for i := startBit; i < bitCount; i++ {
		if nodeNum >= r.metadata.NodeCount {
			break
		}

		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bit := (ip16[byteIdx] >> bitIdx) & 1

		nodeOffset := int(nodeNum) * r.nodeSize
		if nodeOffset+r.nodeSize > len(r.data) {
			return 0, errors.New("node offset out of bounds")
		}

		var record uint32
		switch r.metadata.RecordSize {
		case 24:
			if bit == 0 {
				record = uint32(r.data[nodeOffset])<<16 | uint32(r.data[nodeOffset+1])<<8 | uint32(r.data[nodeOffset+2])
			} else {
				record = uint32(r.data[nodeOffset+3])<<16 | uint32(r.data[nodeOffset+4])<<8 | uint32(r.data[nodeOffset+5])
			}
		case 28:
			if bit == 0 {
				record = (uint32(r.data[nodeOffset+3])&0xf0)<<20 | uint32(r.data[nodeOffset])<<16 | uint32(r.data[nodeOffset+1])<<8 | uint32(r.data[nodeOffset+2])
			} else {
				record = (uint32(r.data[nodeOffset+3])&0x0f)<<24 | uint32(r.data[nodeOffset+4])<<16 | uint32(r.data[nodeOffset+5])<<8 | uint32(r.data[nodeOffset+6])
			}
		case 32:
			if bit == 0 {
				record = binary.BigEndian.Uint32(r.data[nodeOffset : nodeOffset+4])
			} else {
				record = binary.BigEndian.Uint32(r.data[nodeOffset+4 : nodeOffset+8])
			}
		default:
			return 0, fmt.Errorf("unsupported record size: %d", r.metadata.RecordSize)
		}

		if record == r.metadata.NodeCount {
			// Not found
			return 0, nil
		}

		if record > r.metadata.NodeCount {
			// Found data - return offset
			return int(record) - int(r.metadata.NodeCount) + r.dataOffset - 16, nil
		}

		nodeNum = record
	}

	return 0, nil
}

// LookupCountry returns the country code for an IP
func (r *mmdbReader) LookupCountry(ip net.IP) string {
	offset, err := r.lookup(ip)
	if err != nil || offset == 0 {
		return ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	val, _, err := r.decodeValue(r.data, offset)
	if err != nil {
		return ""
	}

	// Handle different database formats
	if m, ok := val.(map[string]interface{}); ok {
		// Country database format
		if country, ok := m["country"].(map[string]interface{}); ok {
			if code, ok := country["iso_code"].(string); ok {
				return code
			}
		}
		// Simpler format (just iso_code at top level)
		if code, ok := m["iso_code"].(string); ok {
			return code
		}
		// Alternative: country_code field
		if code, ok := m["country_code"].(string); ok {
			return code
		}
	}

	// Direct string (some databases)
	if code, ok := val.(string); ok && len(code) == 2 {
		return code
	}

	return ""
}

// LookupASN returns the ASN and organization for an IP
func (r *mmdbReader) LookupASN(ip net.IP) (uint, string) {
	offset, err := r.lookup(ip)
	if err != nil || offset == 0 {
		return 0, ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	val, _, err := r.decodeValue(r.data, offset)
	if err != nil {
		return 0, ""
	}

	if m, ok := val.(map[string]interface{}); ok {
		var asn uint
		var org string

		if v, ok := m["autonomous_system_number"].(uint64); ok {
			asn = uint(v)
		}
		if v, ok := m["autonomous_system_organization"].(string); ok {
			org = v
		}
		// Alternative field names
		if v, ok := m["asn"].(uint64); ok {
			asn = uint(v)
		}
		if v, ok := m["as_org"].(string); ok {
			org = v
		}
		if v, ok := m["name"].(string); ok && org == "" {
			org = v
		}

		return asn, org
	}

	return 0, ""
}

// LookupCity returns city information for an IP
func (r *mmdbReader) LookupCity(ip net.IP) (city, region, postal string, lat, lon float64, tz string) {
	offset, err := r.lookup(ip)
	if err != nil || offset == 0 {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	val, _, err := r.decodeValue(r.data, offset)
	if err != nil {
		return
	}

	if m, ok := val.(map[string]interface{}); ok {
		// City name
		if cityData, ok := m["city"].(map[string]interface{}); ok {
			if names, ok := cityData["names"].(map[string]interface{}); ok {
				if name, ok := names["en"].(string); ok {
					city = name
				}
			}
		}

		// Region/subdivision
		if subdivisions, ok := m["subdivisions"].([]interface{}); ok && len(subdivisions) > 0 {
			if sub, ok := subdivisions[0].(map[string]interface{}); ok {
				if names, ok := sub["names"].(map[string]interface{}); ok {
					if name, ok := names["en"].(string); ok {
						region = name
					}
				}
			}
		}

		// Postal code
		if postalData, ok := m["postal"].(map[string]interface{}); ok {
			if code, ok := postalData["code"].(string); ok {
				postal = code
			}
		}

		// Location
		if location, ok := m["location"].(map[string]interface{}); ok {
			if v, ok := location["latitude"].(float64); ok {
				lat = v
			}
			if v, ok := location["longitude"].(float64); ok {
				lon = v
			}
			if v, ok := location["time_zone"].(string); ok {
				tz = v
			}
		}
	}

	return
}

// Close closes the reader
func (r *mmdbReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = nil
	r.metadata = nil
}
