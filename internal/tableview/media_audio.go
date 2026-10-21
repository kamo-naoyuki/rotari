package tableview

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// probeWAV reads a RIFF or RF64 WAVE file's fmt chunk and the size of its
// data chunk.
func probeWAV(reader headerReader, properties *Properties) {
	header := reader.read(0, 12)
	if len(header) < 12 || (string(header[:4]) != "RIFF" && string(header[:4]) != "RF64") || string(header[8:12]) != "WAVE" {
		return
	}
	properties.Format = "WAV"
	var byteRate, ds64Data uint64
	offset := int64(12)
	// Chunks before the data are small; give up after a bounded scan.
	for chunks := 0; chunks < 64 && offset+8 <= reader.size; chunks++ {
		chunk := reader.read(offset, 8)
		if len(chunk) < 8 {
			return
		}
		id, size := string(chunk[:4]), int64(binary.LittleEndian.Uint32(chunk[4:8]))
		body := offset + 8
		switch id {
		case "fmt ":
			format := reader.read(body, min(size, 40))
			if len(format) < 16 {
				return
			}
			tag := binary.LittleEndian.Uint16(format[0:2])
			properties.Channels = int(binary.LittleEndian.Uint16(format[2:4]))
			properties.SampleRate = int(binary.LittleEndian.Uint32(format[4:8]))
			byteRate = uint64(binary.LittleEndian.Uint32(format[8:12]))
			properties.BitDepth = int(binary.LittleEndian.Uint16(format[14:16]))
			// WAVE_FORMAT_EXTENSIBLE names the real format in its GUID.
			if tag == 0xFFFE && len(format) >= 26 {
				tag = binary.LittleEndian.Uint16(format[24:26])
			}
			properties.Format = wavFormatName(tag)
			properties.Bitrate = int(byteRate * 8)
		case "ds64":
			if values := reader.read(body, 16); len(values) == 16 {
				ds64Data = binary.LittleEndian.Uint64(values[8:16])
			}
		case "data":
			data := uint64(size)
			if ds64Data > 0 && size == 0xFFFFFFFF {
				data = ds64Data
			}
			// A streamed file may leave the size unset or past its end.
			if remaining := uint64(reader.size - body); data > remaining {
				data = remaining
			}
			if byteRate > 0 {
				properties.Duration = float64(data) / float64(byteRate)
			}
			return
		}
		offset = body + size + size%2
	}
}

func wavFormatName(tag uint16) string {
	switch tag {
	case 1:
		return "WAV PCM"
	case 3:
		return "WAV float"
	case 6:
		return "WAV A-law"
	case 7:
		return "WAV µ-law"
	}
	return fmt.Sprintf("WAV (format 0x%04x)", tag)
}

// id3Size is the length of an ID3v2 tag at the start of data, or 0.
func id3Size(data []byte) int64 {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return 0
	}
	size := int64(data[6]&0x7f)<<21 | int64(data[7]&0x7f)<<14 | int64(data[8]&0x7f)<<7 | int64(data[9]&0x7f)
	size += 10
	if data[5]&0x10 != 0 {
		size += 10 // footer
	}
	return size
}

// probeFLAC reads a FLAC stream's STREAMINFO block.
func probeFLAC(reader headerReader, properties *Properties) {
	start := id3Size(reader.read(0, 10))
	header := reader.read(start, 4+4+34)
	if len(header) < 42 || string(header[:4]) != "fLaC" || header[4]&0x7f != 0 {
		return
	}
	properties.Format = "FLAC"
	info := header[8:]
	sampleRate := int(info[10])<<12 | int(info[11])<<4 | int(info[12])>>4
	properties.Channels = int(info[12]>>1&0x07) + 1
	properties.BitDepth = int(info[12]&0x01)<<4 | int(info[13]>>4) + 1
	samples := uint64(info[13]&0x0f)<<32 | uint64(binary.BigEndian.Uint32(info[14:18]))
	properties.SampleRate = sampleRate
	if sampleRate > 0 && samples > 0 {
		properties.Duration = float64(samples) / float64(sampleRate)
	}
}

// MPEG audio header tables, by version (1, 2, 2.5) and layer (I, II, III).
var (
	mpegBitrates = map[[2]int][15]int{
		{1, 1}: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
		{1, 2}: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
		{1, 3}: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
		{2, 1}: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
		{2, 2}: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
		{2, 3}: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
	}
	mpegSampleRates = map[int][3]int{1: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 25: {11025, 12000, 8000}}
)

// mpegFrame is a parsed MPEG audio frame header.
type mpegFrame struct {
	version, layer, bitrate, sampleRate, channels, length, samples int
}

func parseMPEGFrame(header []byte) (mpegFrame, bool) {
	if len(header) < 4 || header[0] != 0xff || header[1]&0xe0 != 0xe0 {
		return mpegFrame{}, false
	}
	var frame mpegFrame
	switch header[1] >> 3 & 0x03 {
	case 0:
		frame.version = 25
	case 2:
		frame.version = 2
	case 3:
		frame.version = 1
	default:
		return mpegFrame{}, false
	}
	frame.layer = 4 - int(header[1]>>1&0x03)
	bitrateIndex, rateIndex := int(header[2]>>4), int(header[2]>>2&0x03)
	if frame.layer == 4 || bitrateIndex == 0 || bitrateIndex == 15 || rateIndex == 3 {
		return mpegFrame{}, false
	}
	tableVersion := min(frame.version, 2)
	frame.bitrate = mpegBitrates[[2]int{tableVersion, frame.layer}][bitrateIndex] * 1000
	frame.sampleRate = mpegSampleRates[frame.version][rateIndex]
	frame.channels = 2
	if header[3]>>6 == 3 {
		frame.channels = 1
	}
	padding := int(header[2] >> 1 & 0x01)
	switch {
	case frame.layer == 1:
		frame.samples = 384
		frame.length = (12*frame.bitrate/frame.sampleRate + padding) * 4
	case frame.layer == 3 && frame.version != 1:
		frame.samples = 576
		frame.length = 72*frame.bitrate/frame.sampleRate + padding
	default:
		frame.samples = 1152
		frame.length = 144*frame.bitrate/frame.sampleRate + padding
	}
	return frame, true
}

// probeMP3 reads the first MPEG audio frame after any ID3v2 tag, and a
// Xing, Info, or VBRI header in it for the frame count; without one the
// stream is taken to be constant-bitrate.
func probeMP3(reader headerReader, properties *Properties) {
	start := id3Size(reader.read(0, 10))
	data := reader.read(start, 64<<10)
	for index := 0; index+4 <= len(data); index++ {
		frame, ok := parseMPEGFrame(data[index:])
		if !ok {
			continue
		}
		// A second frame where the first ends rules out a false sync.
		if next := index + frame.length; next+4 <= len(data) {
			if _, ok := parseMPEGFrame(data[next:]); !ok {
				continue
			}
		}
		properties.Format = []string{"", "MP1", "MP2", "MP3"}[frame.layer]
		properties.SampleRate, properties.Channels = frame.sampleRate, frame.channels
		audioStart := start + int64(index)
		if frames := mpegFrameCount(data[index:], frame); frames > 0 {
			properties.Duration = float64(frames) * float64(frame.samples) / float64(frame.sampleRate)
			return
		}
		audio := reader.size - audioStart
		if tag := reader.read(reader.size-128, 3); string(tag) == "TAG" {
			audio -= 128
		}
		properties.Bitrate = frame.bitrate
		properties.Duration = float64(audio) * 8 / float64(frame.bitrate)
		return
	}
}

// mpegFrameCount is the frame count a Xing, Info, or VBRI header records in
// the first frame, or 0.
func mpegFrameCount(data []byte, frame mpegFrame) int {
	side := 32
	switch {
	case frame.version == 1 && frame.channels == 1:
		side = 17
	case frame.version != 1 && frame.channels == 2:
		side = 17
	case frame.version != 1:
		side = 9
	}
	if xing := 4 + side; len(data) >= xing+12 {
		tag := string(data[xing : xing+4])
		if (tag == "Xing" || tag == "Info") && binary.BigEndian.Uint32(data[xing+4:xing+8])&1 != 0 {
			return int(binary.BigEndian.Uint32(data[xing+8 : xing+12]))
		}
	}
	if vbri := 4 + 32; len(data) >= vbri+18 && string(data[vbri:vbri+4]) == "VBRI" {
		return int(binary.BigEndian.Uint32(data[vbri+14 : vbri+18]))
	}
	return 0
}

// oggPage is the part of an Ogg page header the probe reads.
type oggPage struct {
	granule int64
	serial  uint32
	payload []byte
}

func parseOggPage(data []byte) (oggPage, bool) {
	if len(data) < 27 || string(data[:4]) != "OggS" {
		return oggPage{}, false
	}
	segments := int(data[26])
	if len(data) < 27+segments {
		return oggPage{}, false
	}
	length := 0
	for _, size := range data[27 : 27+segments] {
		length += int(size)
	}
	payload := data[27+segments:]
	payload = payload[:min(length, len(payload))]
	return oggPage{granule: int64(binary.LittleEndian.Uint64(data[6:14])), serial: binary.LittleEndian.Uint32(data[14:18]), payload: payload}, true
}

// probeOgg reads an Ogg stream's first packet, a Vorbis or Opus
// identification header, and the last page's granule position for the
// duration.
func probeOgg(reader headerReader, properties *Properties) {
	first, ok := parseOggPage(reader.read(0, 64<<10))
	if !ok {
		return
	}
	packet := first.payload
	var rate int
	var preSkip int64
	switch {
	case len(packet) >= 30 && string(packet[:7]) == "\x01vorbis":
		properties.Format = "Ogg Vorbis"
		properties.Channels = int(packet[11])
		properties.SampleRate = int(binary.LittleEndian.Uint32(packet[12:16]))
		rate = properties.SampleRate
	case len(packet) >= 19 && string(packet[:8]) == "OpusHead":
		properties.Format = "Ogg Opus"
		properties.Channels = int(packet[9])
		preSkip = int64(binary.LittleEndian.Uint16(packet[10:12]))
		// Opus always decodes at 48 kHz; the header's rate is the input's.
		properties.SampleRate, rate = 48000, 48000
	case len(packet) >= 5 && string(packet[:5]) == "\x7fFLAC":
		properties.Format = "Ogg FLAC"
		return
	default:
		properties.Format = "Ogg"
		return
	}
	// The last page of the stream carries the final granule position.
	tail := reader.read(max(0, reader.size-(256<<10)), 256<<10)
	for index := bytes.LastIndex(tail, []byte("OggS")); index >= 0; index = bytes.LastIndex(tail[:index], []byte("OggS")) {
		page, ok := parseOggPage(tail[index:])
		if ok && page.serial == first.serial && page.granule > 0 {
			properties.Duration = float64(page.granule-preSkip) / float64(rate)
			return
		}
	}
}
