package webui

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// npy builds an .npy file of the given version with a header and raw data.
func npy(version byte, header string, data []byte) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("\x93NUMPY")
	buffer.Write([]byte{version, 0})
	prefix := 10
	if version > 1 {
		prefix = 12
	}
	padding := 64 - (prefix+len(header)+1)%64
	header += strings.Repeat(" ", padding) + "\n"
	if version == 1 {
		_ = binary.Write(&buffer, binary.LittleEndian, uint16(len(header)))
	} else {
		_ = binary.Write(&buffer, binary.LittleEndian, uint32(len(header)))
	}
	buffer.WriteString(header)
	buffer.Write(data)
	return buffer.Bytes()
}

func values(order binary.ByteOrder, items ...any) []byte {
	var buffer bytes.Buffer
	for _, item := range items {
		_ = binary.Write(&buffer, order, item)
	}
	return buffer.Bytes()
}

func TestReadNumpyArray(t *testing.T) {
	many := make([]any, 60)
	for index := range many {
		many[index] = int64(index - 3)
	}
	tests := []struct {
		name string
		file []byte
		want artifactArray
	}{
		{name: "float32 matrix", file: npy(1, "{'descr': '<f4', 'fortran_order': False, 'shape': (2, 2), }", values(binary.LittleEndian, float32(0.5), float32(-1), float32(math.Inf(1)), float32(math.NaN()))),
			want: artifactArray{Dtype: "<f4", Shape: []int64{2, 2}, Size: 4, Values: []string{"0.5", "-1", "+Inf", "NaN"}}},
		{name: "big-endian float64", file: npy(1, "{'descr': '>f8', 'fortran_order': False, 'shape': (1,), }", values(binary.BigEndian, 3.25)),
			want: artifactArray{Dtype: ">f8", Shape: []int64{1}, Size: 1, Values: []string{"3.25"}}},
		{name: "float16", file: npy(1, "{'descr': '<f2', 'fortran_order': False, 'shape': (2,), }", values(binary.LittleEndian, uint16(0x3c00), uint16(0xc000))),
			want: artifactArray{Dtype: "<f2", Shape: []int64{2}, Size: 2, Values: []string{"1", "-2"}}},
		{name: "int64 first values", file: npy(1, "{'descr': '<i8', 'fortran_order': False, 'shape': (60,), }", values(binary.LittleEndian, many...)),
			want: artifactArray{Dtype: "<i8", Shape: []int64{60}, Size: 60, Note: "first 50 of 60 values in memory order"}},
		{name: "int8 and fortran order", file: npy(1, "{'descr': '|i1', 'fortran_order': True, 'shape': (1, 2), }", []byte{0xff, 0x7f}),
			want: artifactArray{Dtype: "|i1", Shape: []int64{1, 2}, Size: 2, FortranOrder: true, Values: []string{"-1", "127"}}},
		{name: "uint16", file: npy(1, "{'descr': '<u2', 'fortran_order': False, 'shape': (1,), }", values(binary.LittleEndian, uint16(65535))),
			want: artifactArray{Dtype: "<u2", Shape: []int64{1}, Size: 1, Values: []string{"65535"}}},
		{name: "bool", file: npy(1, "{'descr': '|b1', 'fortran_order': False, 'shape': (2,), }", []byte{1, 0}),
			want: artifactArray{Dtype: "|b1", Shape: []int64{2}, Size: 2, Values: []string{"true", "false"}}},
		{name: "scalar", file: npy(1, "{'descr': '<i4', 'fortran_order': False, 'shape': (), }", values(binary.LittleEndian, int32(7))),
			want: artifactArray{Dtype: "<i4", Shape: []int64{}, Size: 1, Values: []string{"7"}}},
		{name: "object array is never read", file: npy(1, "{'descr': '|O', 'fortran_order': False, 'shape': (1,), }", []byte("\x80\x04cos\nsystem\n")),
			want: artifactArray{Dtype: "|O", Shape: []int64{1}, Size: 1, Note: "object arrays are pickled; values are not read"}},
		{name: "strings are described only", file: npy(1, "{'descr': '<U5', 'fortran_order': False, 'shape': (1,), }", make([]byte, 20)),
			want: artifactArray{Dtype: "<U5", Shape: []int64{1}, Size: 1, Note: "values are not shown for this dtype"}},
		{name: "structured dtype", file: npy(1, "{'descr': [('a', '<f4')], 'fortran_order': False, 'shape': (3,), }", make([]byte, 12)),
			want: artifactArray{Dtype: "structured", Shape: []int64{3}, Size: 3, Note: "values are not shown for this dtype"}},
		{name: "version 2 header", file: npy(2, "{'descr': '<i2', 'fortran_order': False, 'shape': (1,), }", values(binary.LittleEndian, int16(-5))),
			want: artifactArray{Dtype: "<i2", Shape: []int64{1}, Size: 1, Values: []string{"-5"}}},
		{name: "fewer values than the shape says", file: npy(1, "{'descr': '<i4', 'fortran_order': False, 'shape': (3,), }", values(binary.LittleEndian, int32(1))),
			want: artifactArray{Dtype: "<i4", Shape: []int64{3}, Size: 3, Values: []string{"1"}, Note: "first 1 of 3 values in memory order"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readNumpyArray(bytes.NewReader(test.file))
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "int64 first values" {
				if len(got.Values) != artifactArrayValues || got.Values[0] != "-3" || got.Values[49] != "46" {
					t.Fatalf("values = %v", got.Values)
				}
				got.Values = nil
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("array = %+v, want %+v", got, test.want)
			}
		})
	}
	for name, file := range map[string][]byte{
		"not npy":        []byte("PK\x03\x04 a zip"),
		"bad version":    npy(9, "{'shape': ()}", nil),
		"truncated":      npy(1, "{'descr': '<f4', 'shape': (1,), }", nil)[:12],
		"no shape":       npy(1, "{'descr': '<f4', 'fortran_order': False, }", nil),
		"negative shape": npy(1, "{'descr': '<f4', 'fortran_order': False, 'shape': (-1,), }", nil),
	} {
		if _, err := readNumpyArray(bytes.NewReader(file)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestReadNumpyArchiveAndEndpoint(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, member := range []struct {
		name   string
		method uint16
		file   []byte
	}{
		{name: "weights.npy", method: zip.Deflate, file: npy(1, "{'descr': '<f4', 'fortran_order': False, 'shape': (2,), }", values(binary.LittleEndian, float32(1), float32(2)))},
		{name: "labels.npy", method: zip.Store, file: npy(1, "{'descr': '<i8', 'fortran_order': False, 'shape': (1,), }", values(binary.LittleEndian, int64(9)))},
		{name: "readme.txt", method: zip.Store, file: []byte("not an array")},
		{name: "broken.npy", method: zip.Store, file: []byte("nope")},
	} {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: member.name, Method: member.method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(member.file); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	arrays, err := readNumpyArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := []artifactArray{
		{Name: "weights", Dtype: "<f4", Shape: []int64{2}, Size: 2, Values: []string{"1", "2"}},
		{Name: "labels", Dtype: "<i8", Shape: []int64{1}, Size: 1, Values: []string{"9"}},
		{Name: "broken", Note: "not a .npy file"},
	}
	if !reflect.DeepEqual(arrays.Arrays, want) || arrays.Truncated {
		t.Fatalf("arrays = %+v", arrays)
	}
	if _, err := readNumpyArchive(bytes.NewReader([]byte("not a zip")), 9); err == nil {
		t.Fatal("a non-zip .npz was read")
	}

	// Through the endpoint, under the same entry rules as every preview.
	f := newArtifactFixture(t)
	write(t, filepath.Join(f.work, "many", "arrays.npz"), archive.String())
	options := testOptions(f.baseDir, false)
	recorder := f.get(t, options, "/api/artifact-array", "many", url.Values{"child": {"arrays.npz"}})
	var served artifactArrays
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &served) != nil || len(served.Arrays) != 3 || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("endpoint = %d %s", recorder.Code, recorder.Body.String())
	}
	if bad := f.get(t, options, "/api/artifact-array", "data.csv", nil); bad.Code != http.StatusBadRequest {
		t.Fatalf("a CSV as an array = %d", bad.Code)
	}
	if outside := f.get(t, options, "/api/artifact-array", "secret.txt", nil); outside.Code != http.StatusForbidden {
		t.Fatalf("outside the roots = %d", outside.Code)
	}
}
