package webui

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Limits on reading NumPy files for a preview.
const (
	// artifactArrayValues is how many leading values an array preview shows.
	artifactArrayValues = 50
	// artifactArrayHeader bounds an .npy header, which is normally a few
	// hundred bytes.
	artifactArrayHeader = 64 << 10
	// artifactArrayMembers bounds the arrays listed from one .npz file.
	artifactArrayMembers = 200
)

// artifactArray describes one NumPy array: its dtype, shape, and memory
// order, and its first values in memory order when its dtype is a plain
// number or boolean.
type artifactArray struct {
	Name         string   `json:"name,omitempty"`
	Dtype        string   `json:"dtype"`
	Shape        []int64  `json:"shape"`
	FortranOrder bool     `json:"fortran_order,omitempty"`
	Size         int64    `json:"size"`
	Values       []string `json:"values,omitempty"`
	// Note says why values are missing or limited.
	Note string `json:"note,omitempty"`
}

// artifactArrays is the /api/artifact-array response.
type artifactArrays struct {
	Arrays []artifactArray `json:"arrays"`
	// Truncated reports an .npz file with more arrays than are listed.
	Truncated bool `json:"truncated,omitempty"`
}

// serveArtifactArray describes a .npy file, or each array of a .npz file,
// read from the file's headers; nothing is unpickled or executed.
func (s site) serveArtifactArray(baseDir string, writer http.ResponseWriter, request *http.Request) {
	served, err := s.resolveArtifact(baseDir, request.URL.Query())
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	file, err := served.open()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeWebError(writer, fmt.Errorf("%s is not a regular file", served.name()))
		return
	}
	var arrays artifactArrays
	switch strings.ToLower(filepath.Ext(served.name())) {
	case ".npy":
		var array artifactArray
		array, err = readNumpyArray(file)
		arrays.Arrays = []artifactArray{array}
	case ".npz":
		arrays, err = readNumpyArchive(file, info.Size())
	default:
		err = fmt.Errorf("%s is not a .npy or .npz file", served.name())
	}
	if err != nil {
		writeWebError(writer, err)
		return
	}
	noStore(writer)
	writeWebJSON(writer, arrays)
}

func readNumpyArchive(file io.ReaderAt, size int64) (artifactArrays, error) {
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return artifactArrays{}, fmt.Errorf("not a readable .npz archive")
	}
	result := artifactArrays{Arrays: []artifactArray{}}
	for _, member := range archive.File {
		if !strings.HasSuffix(member.Name, ".npy") {
			continue
		}
		if len(result.Arrays) == artifactArrayMembers {
			result.Truncated = true
			break
		}
		array, err := readNumpyMember(member)
		if err != nil {
			array = artifactArray{Note: err.Error()}
		}
		array.Name = strings.TrimSuffix(member.Name, ".npy")
		result.Arrays = append(result.Arrays, array)
	}
	return result, nil
}

func readNumpyMember(member *zip.File) (artifactArray, error) {
	reader, err := member.Open()
	if err != nil {
		return artifactArray{}, err
	}
	defer reader.Close()
	return readNumpyArray(reader)
}

var (
	numpyDescr   = regexp.MustCompile(`'descr'\s*:\s*'([^']*)'`)
	numpyFortran = regexp.MustCompile(`'fortran_order'\s*:\s*(True|False)`)
	numpyShape   = regexp.MustCompile(`'shape'\s*:\s*\(([^)]*)\)`)
)

// readNumpyArray reads an .npy stream's header and, for a plain numeric or
// boolean dtype, its first values. The header is a Python dict literal,
// matched as text rather than evaluated.
func readNumpyArray(reader io.Reader) (artifactArray, error) {
	prefix := make([]byte, 8)
	if _, err := io.ReadFull(reader, prefix); err != nil || !bytes.Equal(prefix[:6], []byte("\x93NUMPY")) {
		return artifactArray{}, errors.New("not a .npy file")
	}
	var length int
	switch prefix[6] {
	case 1:
		var short uint16
		if err := binary.Read(reader, binary.LittleEndian, &short); err != nil {
			return artifactArray{}, errors.New("truncated .npy header")
		}
		length = int(short)
	case 2, 3:
		var long uint32
		if err := binary.Read(reader, binary.LittleEndian, &long); err != nil {
			return artifactArray{}, errors.New("truncated .npy header")
		}
		if long > artifactArrayHeader {
			return artifactArray{}, errors.New(".npy header is too large")
		}
		length = int(long)
	default:
		return artifactArray{}, fmt.Errorf("unsupported .npy version %d", prefix[6])
	}
	header := make([]byte, length)
	if _, err := io.ReadFull(reader, header); err != nil {
		return artifactArray{}, errors.New("truncated .npy header")
	}
	text := string(header)
	array := artifactArray{Shape: []int64{}, Size: 1}
	if match := numpyDescr.FindStringSubmatch(text); match != nil {
		array.Dtype = match[1]
	} else {
		// A structured dtype is a list of fields, not a string.
		array.Dtype = "structured"
	}
	if match := numpyFortran.FindStringSubmatch(text); match != nil {
		array.FortranOrder = match[1] == "True"
	}
	match := numpyShape.FindStringSubmatch(text)
	if match == nil {
		return artifactArray{}, errors.New(".npy header has no shape")
	}
	for _, part := range strings.Split(match[1], ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		dimension, err := strconv.ParseInt(strings.TrimSuffix(part, "L"), 10, 64)
		if err != nil || dimension < 0 {
			return artifactArray{}, errors.New(".npy header has an invalid shape")
		}
		array.Shape = append(array.Shape, dimension)
		array.Size *= dimension
	}
	array.Values, array.Note = readNumpyValues(reader, array.Dtype, array.Size)
	return array, nil
}

// readNumpyValues formats up to artifactArrayValues leading values of a
// boolean, integer, unsigned, or floating dtype. Object arrays hold pickles,
// which are never read, and other dtypes are described only.
func readNumpyValues(reader io.Reader, dtype string, size int64) ([]string, string) {
	if dtype == "|O" || strings.HasSuffix(dtype, "O") {
		return nil, "object arrays are pickled; values are not read"
	}
	if len(dtype) < 3 {
		return nil, "values are not shown for this dtype"
	}
	var order binary.ByteOrder = binary.LittleEndian
	if dtype[0] == '>' {
		order = binary.BigEndian
	}
	kind := dtype[1]
	width, err := strconv.Atoi(dtype[2:])
	if err != nil || !strings.ContainsRune("biuf", rune(kind)) || (kind == 'f' && width != 2 && width != 4 && width != 8) ||
		(kind != 'f' && width != 1 && width != 2 && width != 4 && width != 8) {
		return nil, "values are not shown for this dtype"
	}
	count := min(size, artifactArrayValues)
	data := make([]byte, int(count)*width)
	read, _ := io.ReadFull(reader, data)
	values := make([]string, 0, count)
	for offset := 0; offset+width <= read; offset += width {
		values = append(values, formatNumpyValue(order, kind, data[offset:offset+width]))
	}
	note := ""
	if int64(len(values)) < size {
		note = fmt.Sprintf("first %d of %d values in memory order", len(values), size)
	}
	return values, note
}

func formatNumpyValue(order binary.ByteOrder, kind byte, data []byte) string {
	var bits uint64
	switch len(data) {
	case 1:
		bits = uint64(data[0])
	case 2:
		bits = uint64(order.Uint16(data))
	case 4:
		bits = uint64(order.Uint32(data))
	case 8:
		bits = order.Uint64(data)
	}
	switch kind {
	case 'b':
		return strconv.FormatBool(bits != 0)
	case 'u':
		return strconv.FormatUint(bits, 10)
	case 'i':
		shift := 64 - 8*len(data)
		return strconv.FormatInt(int64(bits<<shift)>>shift, 10)
	}
	switch len(data) {
	case 2:
		return strconv.FormatFloat(float64(halfToFloat(uint16(bits))), 'g', -1, 32)
	case 4:
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(bits))), 'g', -1, 32)
	}
	return strconv.FormatFloat(math.Float64frombits(bits), 'g', -1, 64)
}

// halfToFloat converts an IEEE 754 half-precision value.
func halfToFloat(half uint16) float32 {
	sign := uint32(half>>15) << 31
	exponent := uint32(half>>10) & 0x1f
	fraction := uint32(half) & 0x3ff
	switch {
	case exponent == 0 && fraction == 0:
		return math.Float32frombits(sign)
	case exponent == 0:
		value := float32(fraction) / 1024 * float32(math.Pow(2, -14))
		if sign != 0 {
			return -value
		}
		return value
	case exponent == 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | fraction<<13)
	}
	return math.Float32frombits(sign | (exponent+112)<<23 | fraction<<13)
}
