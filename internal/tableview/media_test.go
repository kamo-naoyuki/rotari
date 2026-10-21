package tableview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func probeFile(t *testing.T, path string) Properties {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return Probe(file, info.Size(), filepath.Ext(path))
}

// The files in testdata/media were made with ffmpeg; each case's values are
// what ffprobe reports for it.
func TestProbeReadsEncodedFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		want Properties
		// slack is the allowed duration error: an MP3 without a frame count
		// is estimated from its size.
		slack float64
	}{
		{"pcm16.wav", Properties{Format: "WAV PCM", Duration: 0.5, SampleRate: 16000, Channels: 1, BitDepth: 16, Bitrate: 256000}, 0.001},
		{"float.wav", Properties{Format: "WAV float", Duration: 0.250023, SampleRate: 22050, Channels: 2, BitDepth: 32, Bitrate: 1411200}, 0.001},
		{"tone.flac", Properties{Format: "FLAC", Duration: 0.3, SampleRate: 44100, Channels: 2, BitDepth: 24}, 0.001},
		{"cbr.mp3", Properties{Format: "MP3", Duration: 1.044875, SampleRate: 44100, Channels: 2, Bitrate: 128000}, 0.03},
		{"vbr.mp3", Properties{Format: "MP3", Duration: 1.032, SampleRate: 48000, Channels: 2}, 0.03},
		{"mpeg2.mp3", Properties{Format: "MP3", Duration: 0.862, SampleRate: 22050, Channels: 1, Bitrate: 32000}, 0.03},
		{"vorbis.ogg", Properties{Format: "Ogg Vorbis", Duration: 0.6, SampleRate: 32000, Channels: 1}, 0.001},
		{"speech.opus", Properties{Format: "Ogg Opus", Duration: 0.4, SampleRate: 48000, Channels: 2}, 0.01},
		{"aac.m4a", Properties{Format: "M4A (AAC)", Duration: 0.7, SampleRate: 24000, Channels: 1}, 0.01},
		{"alac.m4a", Properties{Format: "M4A (ALAC)", Duration: 0.5, SampleRate: 44100, Channels: 2, BitDepth: 16}, 0.01},
		{"clip.mp4", Properties{Format: "MP4 (H.264, AAC)", Duration: 0.5, SampleRate: 44100, Channels: 1, Width: 32, Height: 24}, 0.03},
		{"clip.mov", Properties{Format: "MOV (H.264)", Duration: 0.3, Width: 40, Height: 30}, 0.01},
		{"clip.webm", Properties{Format: "WEBM (VP9, Opus)", Duration: 0.508, SampleRate: 48000, Channels: 1, Width: 48, Height: 32}, 0.01},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := probeFile(t, filepath.Join("testdata", "media", test.name))
			if math.Abs(got.Duration-test.want.Duration) > test.slack {
				t.Errorf("duration = %v, want %v", got.Duration, test.want.Duration)
			}
			wantBitrate := test.want.Bitrate
			if wantBitrate == 0 {
				wantBitrate = int(float64(got.Size) * 8 / got.Duration)
			}
			got.Duration, test.want.Duration = 0, 0
			test.want.Size, test.want.Bitrate = got.Size, wantBitrate
			if got != test.want {
				t.Errorf("properties = %+v\n          want %+v", got, test.want)
			}
		})
	}
}

// wavHeader writes a canonical WAV header for data bytes of 16-bit PCM.
func wavHeader(riff string, format uint16, channels, rate int, dataSize uint32, extra []byte) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(riff)
	binary.Write(&buffer, binary.LittleEndian, uint32(0))
	buffer.WriteString("WAVE")
	buffer.Write(extra)
	fmtBody := new(bytes.Buffer)
	for _, field := range []any{format, uint16(channels), uint32(rate), uint32(rate * channels * 2), uint16(channels * 2), uint16(16)} {
		binary.Write(fmtBody, binary.LittleEndian, field)
	}
	if format == 0xFFFE {
		// cbSize, valid bits, channel mask, and the GUID naming PCM.
		for _, field := range []any{uint16(22), uint16(16), uint32(0), uint16(1)} {
			binary.Write(fmtBody, binary.LittleEndian, field)
		}
		fmtBody.Write(make([]byte, 14))
	}
	buffer.WriteString("fmt ")
	binary.Write(&buffer, binary.LittleEndian, uint32(fmtBody.Len()))
	buffer.Write(fmtBody.Bytes())
	// An odd-sized chunk before the data is padded to an even length.
	buffer.WriteString("LIST")
	binary.Write(&buffer, binary.LittleEndian, uint32(3))
	buffer.WriteString("abc\x00")
	buffer.WriteString("data")
	binary.Write(&buffer, binary.LittleEndian, dataSize)
	return buffer.Bytes()
}

func probeBytes(data []byte, extension string) Properties {
	return Probe(bytes.NewReader(data), int64(len(data)), extension)
}

func TestProbeWAVVariants(t *testing.T) {
	samples := make([]byte, 32000) // a second of 16 kHz mono
	for _, test := range []struct {
		name   string
		header []byte
		want   string
	}{
		{"extensible", wavHeader("RIFF", 0xFFFE, 1, 16000, 32000, nil), "WAV PCM"},
		// A stream writer may leave the size unset; the file's end bounds it.
		{"streamed", wavHeader("RIFF", 1, 1, 16000, 0xFFFFFFFF, nil), "WAV PCM"},
		{"rf64", wavHeader("RF64", 1, 1, 16000, 0xFFFFFFFF, append([]byte("ds64\x1c\x00\x00\x00"), append(binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint64(nil, 0), 32000), make([]byte, 12)...)...)), "WAV PCM"},
	} {
		got := probeBytes(append(test.header, samples...), ".wav")
		if got.Format != test.want || got.Duration != 1 || got.SampleRate != 16000 || got.Channels != 1 {
			t.Errorf("%s = %+v", test.name, got)
		}
	}
}

func TestProbeKeepsFormatAndSizeOfUnreadableFiles(t *testing.T) {
	for extension, data := range map[string][]byte{
		".wav": []byte("RIFF"), ".flac": []byte("fLaC"), ".mp3": bytes.Repeat([]byte{0xff}, 100),
		".ogg": []byte("OggS"), ".m4a": []byte("\x00\x00\x00\x08moov"), ".webm": []byte{0x1a}, ".png": []byte("\x89PNG"), ".svg": []byte("<svg"),
	} {
		got := probeBytes(data, extension)
		if got.Size != int64(len(data)) || got.Format == "" || got.Duration != 0 || got.Width != 0 {
			t.Errorf("%s = %+v", extension, got)
		}
	}
}

func TestProbeReadsImageSizes(t *testing.T) {
	picture := image.NewPaletted(image.Rect(0, 0, 7, 5), color.Palette{color.Black, color.White})
	var encoded = map[string]*bytes.Buffer{".png": {}, ".jpg": {}, ".gif": {}}
	if err := png.Encode(encoded[".png"], picture); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(encoded[".jpg"], picture, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(encoded[".gif"], picture, nil); err != nil {
		t.Fatal(err)
	}
	for extension, want := range map[string]string{".png": "PNG", ".jpg": "JPEG", ".gif": "GIF"} {
		got := probeBytes(encoded[extension].Bytes(), extension)
		if got.Format != want || got.Width != 7 || got.Height != 5 || got.Bitrate != 0 {
			t.Errorf("%s = %+v", extension, got)
		}
	}
	for svg, want := range map[string][2]int{
		`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120px" height="80">`: {120, 80},
		`<svg viewBox="0 0 64.4 32" width="100%">`:                                                {64, 32},
		`<!-- comment --><svg>`: {0, 0},
	} {
		got := probeBytes([]byte(svg), ".svg")
		if got.Format != "SVG" || got.Width != want[0] || got.Height != want[1] {
			t.Errorf("%s = %+v", svg, got)
		}
	}
}

func TestPagesDescribeTheMediaCellsName(t *testing.T) {
	root, other, outside := t.TempDir(), t.TempDir(), t.TempDir()
	wav, err := os.ReadFile(filepath.Join("testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "data", "wav", "a.wav"), string(wav))
	writeFile(t, filepath.Join(root, "shared", "b.wav"), string(wav))
	writeFile(t, filepath.Join(other, "c.wav"), string(wav))
	writeFile(t, filepath.Join(outside, "secret.wav"), string(wav))
	if err := os.Symlink(filepath.Join(outside, "secret.wav"), filepath.Join(root, "data", "link.wav")); err != nil {
		t.Fatal(err)
	}
	csv := "path\nwav/a.wav\n" + filepath.Join(other, "c.wav") + "\n" + filepath.Join(outside, "secret.wav") + "\nwav/gone.wav\nlink.wav\nnotes.txt\n../shared/b.wav\n"
	writeFile(t, filepath.Join(root, "data", "list.csv"), csv)
	target := Target{Path: filepath.Join(root, "data", "list.csv"), Roots: []string{root, other},
		Bases: []Base{{ID: "shared", Label: "shared", Path: filepath.Join(root, "shared")}}}
	table, err := OpenFile(target, Format{Separator: ',', Header: true})
	if err != nil {
		t.Fatal(err)
	}
	// Relative paths resolve from the file's directory here; the default
	// base is the caller's first.
	page, err := table.Page(Query{Base: FileBase})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "", "Not shown: outside the allowed roots", "File not found", "Not shown: a symlink leaves the allowed roots", "-", ""}
	for index, row := range page.Rows {
		media := row.Media[0]
		if want[index] == "-" {
			if media != nil {
				t.Errorf("row %d: %q is not media: %+v", index+1, row.Cells[0], media)
			}
			continue
		}
		if media == nil || media.Kind != "audio" || media.Ref != row.Cells[0] || media.Error != want[index] {
			t.Errorf("row %d: media = %+v, want error %q", index+1, media, want[index])
			continue
		}
		if want[index] == "" && (media.Properties == nil || media.Properties.Format != "WAV PCM" || media.Properties.Duration != 0.5) {
			t.Errorf("row %d: properties = %+v", index+1, media.Properties)
		}
	}
	// From the caller's base, the same relative paths name other files.
	page, err = table.Page(Query{Base: "shared", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if media := page.Rows[0].Media[0]; media == nil || media.Error != "File not found" || media.Base != "shared" {
		t.Errorf("wav/a.wav from shared = %+v", media)
	}
	if _, err := table.Page(Query{Base: "nowhere"}); err == nil {
		t.Error("an unknown base was accepted")
	}

	// The media a cell names is opened under the roots only, and only if
	// it is an image, audio, or video file.
	path, err := target.ResolveMedia(false, FileBase, "wav/a.wav")
	if err != nil || path != filepath.Join(root, "data", "wav", "a.wav") {
		t.Fatalf("resolved %q, %v", path, err)
	}
	for _, ref := range []string{"list.csv", filepath.Join(outside, "secret.wav"), "../../../" + filepath.Base(outside) + "/secret.wav", ""} {
		if path, err := target.ResolveMedia(false, FileBase, ref); err == nil {
			t.Errorf("%q resolved to %s", ref, path)
		}
	}
	if file, err := target.OpenMedia(filepath.Join(root, "data", "link.wav")); err == nil {
		file.Close()
		t.Error("a symlink out of the root was opened")
	}
}
