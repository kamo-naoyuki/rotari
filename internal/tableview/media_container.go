package tableview

import (
	"encoding/binary"
	"math"
)

// mp4Box is one box of an ISO base media (MP4, M4A, MOV) file.
type mp4Box struct {
	kind string
	// body is the box's contents after its header.
	body []byte
}

// mp4Boxes splits data into the boxes it holds.
func mp4Boxes(data []byte) []mp4Box {
	var boxes []mp4Box
	for len(data) >= 8 {
		size := int64(binary.BigEndian.Uint32(data[:4]))
		header := int64(8)
		switch size {
		case 0:
			size = int64(len(data))
		case 1:
			if len(data) < 16 {
				return boxes
			}
			size, header = int64(binary.BigEndian.Uint64(data[8:16])), 16
		}
		if size < header || size > int64(len(data)) {
			return boxes
		}
		boxes = append(boxes, mp4Box{kind: string(data[4:8]), body: data[header:size]})
		data = data[size:]
	}
	return boxes
}

func mp4Child(boxes []mp4Box, path ...string) (mp4Box, bool) {
	for _, box := range boxes {
		if box.kind != path[0] {
			continue
		}
		if len(path) == 1 {
			return box, true
		}
		return mp4Child(mp4Boxes(box.body), path[1:]...)
	}
	return mp4Box{}, false
}

// mp4Moov finds the top-level moov box, which may follow the media data,
// and reads it.
func mp4Moov(reader headerReader) []byte {
	for offset := int64(0); offset+8 <= reader.size; {
		header := reader.read(offset, 16)
		if len(header) < 8 {
			return nil
		}
		size, length := int64(binary.BigEndian.Uint32(header[:4])), int64(8)
		switch {
		case size == 1 && len(header) == 16:
			size, length = int64(binary.BigEndian.Uint64(header[8:16])), 16
		case size == 0:
			size = reader.size - offset
		}
		if size < length {
			return nil
		}
		if string(header[4:8]) == "moov" {
			return reader.read(offset+length, size-length)
		}
		offset += size
	}
	return nil
}

// probeMP4 reads the movie header's duration and, from the tracks, the
// first audio track's codec, channels, and rate and the first video
// track's size.
func probeMP4(reader headerReader, properties *Properties) {
	moov := mp4Boxes(mp4Moov(reader))
	if mvhd, ok := mp4Child(moov, "mvhd"); ok {
		properties.Duration = mp4Duration(mvhd.body)
	}
	codecs := map[string]string{"mp4a": "AAC", "alac": "ALAC", "Opus": "Opus", "fLaC": "FLAC", "ac-3": "AC-3", "ec-3": "E-AC-3",
		"avc1": "H.264", "avc3": "H.264", "hvc1": "HEVC", "hev1": "HEVC", "av01": "AV1", "vp09": "VP9", "mp4v": "MPEG-4"}
	var audio, video string
	for _, trak := range moov {
		if trak.kind != "trak" {
			continue
		}
		children := mp4Boxes(trak.body)
		hdlr, ok := mp4Child(children, "mdia", "hdlr")
		if !ok || len(hdlr.body) < 12 {
			continue
		}
		stsd, ok := mp4Child(children, "mdia", "minf", "stbl", "stsd")
		if !ok || len(stsd.body) < 16 {
			continue
		}
		// stsd: version and flags, entry count, then the first entry.
		entry := stsd.body[8:]
		entrySize, kind := int(binary.BigEndian.Uint32(entry[:4])), string(entry[4:8])
		codec := codecs[kind]
		if codec == "" {
			codec = kind
		}
		switch string(hdlr.body[8:12]) {
		case "soun":
			if audio != "" || entrySize < 36 || len(entry) < 36 {
				continue
			}
			audio = codec
			properties.Channels = int(binary.BigEndian.Uint16(entry[24:26]))
			properties.BitDepth = int(binary.BigEndian.Uint16(entry[26:28]))
			properties.SampleRate = int(binary.BigEndian.Uint32(entry[32:36]) >> 16)
			// A lossy codec's sample size field is a placeholder, and AAC's
			// channel count is in its decoder configuration.
			if codec != "ALAC" && codec != "FLAC" {
				properties.BitDepth = 0
			}
			if esds, ok := mp4Child(mp4Boxes(entry[36:min(entrySize, len(entry))]), "esds"); ok && codec == "AAC" {
				if channels := aacChannels(esds.body); channels > 0 {
					properties.Channels = channels
				}
			}
		case "vide":
			if video != "" {
				continue
			}
			video = codec
			if tkhd, ok := mp4Child(children, "tkhd"); ok && len(tkhd.body) >= 84 {
				size := tkhd.body[len(tkhd.body)-8:]
				properties.Width = int(binary.BigEndian.Uint32(size[:4]) >> 16)
				properties.Height = int(binary.BigEndian.Uint32(size[4:]) >> 16)
			}
		}
	}
	switch {
	case video != "" && audio != "":
		properties.Format += " (" + video + ", " + audio + ")"
	case video != "":
		properties.Format += " (" + video + ")"
	case audio != "":
		properties.Format += " (" + audio + ")"
	}
}

// aacChannels reads the channel configuration from an esds box's
// AudioSpecificConfig, or 0 when it is not there.
func aacChannels(body []byte) int {
	if len(body) < 4 {
		return 0
	}
	data := body[4:]
	// descriptor reads a tag and its size, whose bytes carry 7 bits each.
	descriptor := func() (byte, []byte) {
		if len(data) < 2 {
			return 0, nil
		}
		tag, size, index := data[0], 0, 1
		for ; index < len(data) && index <= 4; index++ {
			size = size<<7 | int(data[index]&0x7f)
			if data[index]&0x80 == 0 {
				break
			}
		}
		index++
		if index > len(data) || size > len(data)-index {
			return 0, nil
		}
		return tag, data[index : index+size]
	}
	tag, contents := descriptor()
	if tag != 0x03 || len(contents) < 3 {
		return 0
	}
	// ES_Descriptor: ID, then flags naming optional fields to skip.
	flags := contents[2]
	data = contents[3:]
	if flags&0x80 != 0 && len(data) >= 2 {
		data = data[2:]
	}
	if flags&0x40 != 0 && len(data) >= 1 {
		data = data[min(int(data[0])+1, len(data)):]
	}
	if flags&0x20 != 0 && len(data) >= 2 {
		data = data[2:]
	}
	tag, contents = descriptor()
	if tag != 0x04 || len(contents) < 13 {
		return 0
	}
	data = contents[13:]
	tag, config := descriptor()
	if tag != 0x05 || len(config) < 2 {
		return 0
	}
	// AudioSpecificConfig: object type (5 bits, 6 more when 31),
	// frequency index (4 bits, 24 bits of rate when 15), then the channel
	// configuration (4 bits).
	position := 0
	read := func(count int) int {
		value := 0
		for ; count > 0; count-- {
			if position/8 >= len(config) {
				return -1
			}
			value = value<<1 | int(config[position/8]>>(7-position%8)&1)
			position++
		}
		return value
	}
	if read(5) == 31 {
		read(6)
	}
	if read(4) == 15 {
		read(24)
	}
	return max(read(4), 0)
}

// mp4Duration reads seconds from an mvhd box.
func mp4Duration(body []byte) float64 {
	if len(body) < 20 {
		return 0
	}
	var scale, duration uint64
	if body[0] == 1 {
		if len(body) < 32 {
			return 0
		}
		scale, duration = uint64(binary.BigEndian.Uint32(body[20:24])), binary.BigEndian.Uint64(body[24:32])
	} else {
		scale, duration = uint64(binary.BigEndian.Uint32(body[12:16])), uint64(binary.BigEndian.Uint32(body[16:20]))
	}
	if scale == 0 || duration == math.MaxUint32 || duration == math.MaxUint64 {
		return 0
	}
	return float64(duration) / float64(scale)
}

// EBML element IDs a WebM file's duration is read from.
const (
	ebmlSegment       = 0x18538067
	ebmlInfo          = 0x1549A966
	ebmlTimecodeScale = 0x2AD7B1
	ebmlDuration      = 0x4489
	ebmlTracks        = 0x1654AE6B
	ebmlTrackEntry    = 0xAE
	ebmlTrackType     = 0x83
	ebmlCodecID       = 0x86
	ebmlAudio         = 0xE1
	ebmlSampleRate    = 0xB5
	ebmlChannels      = 0x9F
	ebmlVideo         = 0xE0
	ebmlPixelWidth    = 0xB0
	ebmlPixelHeight   = 0xBA
)

// ebmlVint reads a variable-length integer; keepMarker keeps the length
// bit, as element IDs do. It returns the value, its length, and whether the
// value is the reserved "unknown size".
func ebmlVint(data []byte, keepMarker bool) (uint64, int, bool) {
	if len(data) == 0 || data[0] == 0 {
		return 0, 0, false
	}
	length := 1
	for mask := byte(0x80); data[0]&mask == 0; mask >>= 1 {
		length++
	}
	if len(data) < length {
		return 0, 0, false
	}
	value := uint64(data[0])
	if !keepMarker {
		value &= uint64(0xff >> length)
	}
	unknown := value == uint64(0xff>>length)
	for _, next := range data[1:length] {
		value = value<<8 | uint64(next)
		unknown = unknown && next == 0xff
	}
	return value, length, unknown && !keepMarker
}

// ebmlElement is one element: its ID and contents.
type ebmlElement struct {
	id   uint64
	body []byte
}

// ebmlElements splits data into elements; an element of unknown size runs
// to the end of data.
func ebmlElements(data []byte) []ebmlElement {
	var elements []ebmlElement
	for len(data) > 0 {
		id, idLength, _ := ebmlVint(data, true)
		if idLength == 0 {
			return elements
		}
		size, sizeLength, unknown := ebmlVint(data[idLength:], false)
		if sizeLength == 0 {
			return elements
		}
		start := idLength + sizeLength
		end := len(data)
		if !unknown && size <= uint64(len(data)-start) {
			end = start + int(size)
		}
		elements = append(elements, ebmlElement{id: id, body: data[start:end]})
		data = data[end:]
	}
	return elements
}

func ebmlUint(body []byte) uint64 {
	var value uint64
	for _, next := range body {
		value = value<<8 | uint64(next)
	}
	return value
}

func ebmlFloat(body []byte) float64 {
	switch len(body) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(body)))
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(body))
	}
	return 0
}

// probeWebM reads a WebM (Matroska) file's segment info and tracks, which
// precede its clusters.
func probeWebM(reader headerReader, properties *Properties) {
	for _, top := range ebmlElements(reader.read(0, 1<<20)) {
		if top.id != ebmlSegment {
			continue
		}
		var audio, video string
		for _, element := range ebmlElements(top.body) {
			switch element.id {
			case ebmlInfo:
				scale, duration := uint64(1_000_000), 0.0
				for _, field := range ebmlElements(element.body) {
					switch field.id {
					case ebmlTimecodeScale:
						scale = ebmlUint(field.body)
					case ebmlDuration:
						duration = ebmlFloat(field.body)
					}
				}
				properties.Duration = duration * float64(scale) / 1e9
			case ebmlTracks:
				for _, entry := range ebmlElements(element.body) {
					if entry.id != ebmlTrackEntry {
						continue
					}
					probeWebMTrack(entry.body, properties, &audio, &video)
				}
			}
		}
		switch {
		case video != "" && audio != "":
			properties.Format += " (" + video + ", " + audio + ")"
		case video != "":
			properties.Format += " (" + video + ")"
		case audio != "":
			properties.Format += " (" + audio + ")"
		}
		return
	}
}

func probeWebMTrack(body []byte, properties *Properties, audio, video *string) {
	var kind uint64
	var codec string
	var settings []byte
	for _, field := range ebmlElements(body) {
		switch field.id {
		case ebmlTrackType:
			kind = ebmlUint(field.body)
		case ebmlCodecID:
			codec = string(field.body)
		case ebmlAudio, ebmlVideo:
			settings = field.body
		}
	}
	names := map[string]string{"A_OPUS": "Opus", "A_VORBIS": "Vorbis", "V_VP8": "VP8", "V_VP9": "VP9", "V_AV1": "AV1"}
	if name := names[codec]; name != "" {
		codec = name
	}
	switch {
	case kind == 2 && *audio == "":
		*audio = codec
		for _, field := range ebmlElements(settings) {
			switch field.id {
			case ebmlSampleRate:
				properties.SampleRate = int(ebmlFloat(field.body))
			case ebmlChannels:
				properties.Channels = int(ebmlUint(field.body))
			}
		}
	case kind == 1 && *video == "":
		*video = codec
		for _, field := range ebmlElements(settings) {
			switch field.id {
			case ebmlPixelWidth:
				properties.Width = int(ebmlUint(field.body))
			case ebmlPixelHeight:
				properties.Height = int(ebmlUint(field.body))
			}
		}
	}
}
