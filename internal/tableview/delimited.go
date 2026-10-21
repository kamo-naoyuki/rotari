package tableview

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Format is how a delimited file splits into rows and cells: records
// separated by newlines and cells by Separator, with double quotes around a
// cell that holds either, as in CSV. Blank lines are skipped.
type Format struct {
	Separator rune
	// Header makes the first record the column names instead of a row.
	Header bool
}

func (format Format) validate() error {
	switch separator := format.Separator; {
	case separator == 0:
		return fmt.Errorf("a separator is required")
	case separator == '"' || separator == '\r' || separator == '\n' || !utf8.ValidRune(separator) || separator == utf8.RuneError:
		return fmt.Errorf("the separator must be one character other than a double quote or a line break")
	}
	return nil
}

// checkpointInterval is how many records lie between two offsets the index
// keeps; reading a record skips at most this many.
const checkpointInterval = 256

// maxRecordBytes bounds one record, so an unclosed quote cannot read the
// rest of a large file into memory. Tests lower it.
var maxRecordBytes int64 = 16 << 20

var errRecordTooLong = fmt.Errorf("a record is longer than %d MiB; check the separator and quoting", maxRecordBytes>>20)

// delimitedIndex locates records in one version of a file.
type delimitedIndex struct {
	// checkpoints[k] is the offset of record k*checkpointInterval.
	checkpoints []int64
	records     int
	// fields is the most cells any record has.
	fields int
	first  []string
}

// delimitedTable is a delimited file read from disk a page at a time.
type delimitedTable struct {
	target Target
	format Format
	id     string
	index  *delimitedIndex
}

func openDelimited(target Target, format Format) (*delimitedTable, error) {
	if err := format.validate(); err != nil {
		return nil, err
	}
	info, err := target.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", target.Path)
	}
	// The index depends on the file's version and the separator; whether
	// the first record is a header only changes how rows are numbered.
	indexKey := fmt.Sprintf("index\x00%s\x00%d\x00%d\x00%d", target.Path, info.Size(), info.ModTime().UnixNano(), format.Separator)
	value, err := results.get(indexKey, func() (any, error) { return buildDelimitedIndex(target, format) })
	if err != nil {
		return nil, err
	}
	return &delimitedTable{target: target, format: format, index: value.(*delimitedIndex),
		id: fmt.Sprintf("%s\x00%t", indexKey, format.Header)}, nil
}

func buildDelimitedIndex(target Target, format Format) (*delimitedIndex, error) {
	file, err := target.open(target.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	index := &delimitedIndex{}
	reader := newRecordReader(file, 0, 0, format)
	for {
		if reader.next%checkpointInterval == 0 {
			index.checkpoints = append(index.checkpoints, reader.offset())
		}
		record, err := reader.read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if index.records == 0 {
			index.first = record
		}
		index.records++
		index.fields = max(index.fields, len(record))
	}
	return index, nil
}

func (table *delimitedTable) key() string { return table.id }

func (table *delimitedTable) headerRecords() int {
	if table.format.Header && table.index.records > 0 {
		return 1
	}
	return 0
}

func (table *delimitedTable) columns() []Column {
	columns := make([]Column, table.index.fields)
	for index := range columns {
		columns[index].Name = fmt.Sprintf("column %d", index+1)
		if table.format.Header && index < len(table.index.first) && strings.TrimSpace(table.index.first[index]) != "" {
			columns[index].Name = table.index.first[index]
		}
	}
	return columns
}

func (table *delimitedTable) len() int { return table.index.records - table.headerRecords() }

// reader opens the file positioned at data row number.
func (table *delimitedTable) reader(number int) (*recordReader, error) {
	record := number + table.headerRecords()
	checkpoint := min(record/checkpointInterval, len(table.index.checkpoints)-1)
	file, err := table.target.open(table.target.Path)
	if err != nil {
		return nil, err
	}
	offset := table.index.checkpoints[checkpoint]
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	reader := newRecordReader(file, offset, checkpoint*checkpointInterval, table.format)
	if err := reader.skipTo(record); err != nil {
		file.Close()
		return nil, err
	}
	return reader, nil
}

func (table *delimitedTable) scan(visit func(number int, cells []string) bool) error {
	if table.len() <= 0 {
		return nil
	}
	reader, err := table.reader(0)
	if err != nil {
		return err
	}
	defer reader.file.Close()
	for number := 0; ; number++ {
		record, err := reader.read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !visit(number, record) {
			return nil
		}
	}
}

// rows reads the data rows numbered, which ascend: one run of nearby rows
// is read in sequence, and a far one from its checkpoint.
func (table *delimitedTable) rows(numbers []int) ([][]string, error) {
	rows := make([][]string, 0, len(numbers))
	var reader *recordReader
	defer func() {
		if reader != nil {
			reader.file.Close()
		}
	}()
	for _, number := range numbers {
		record := number + table.headerRecords()
		if reader == nil || record < reader.next || record-reader.next >= checkpointInterval {
			if reader != nil {
				reader.file.Close()
			}
			var err error
			if reader, err = table.reader(number); err != nil {
				return nil, err
			}
		}
		if err := reader.skipTo(record); err != nil {
			return nil, err
		}
		cells, err := reader.read()
		if err != nil {
			return nil, err
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

// recordReader reads records from an offset whose record number is known.
type recordReader struct {
	file  *os.File
	base  int64
	guard *recordGuard
	csv   *csv.Reader
	// next is the number of the record the next read returns.
	next int
}

func newRecordReader(file *os.File, offset int64, record int, format Format) *recordReader {
	guard := &recordGuard{reader: file}
	reader := csv.NewReader(guard)
	reader.Comma = format.Separator
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	return &recordReader{file: file, base: offset, guard: guard, csv: reader, next: record}
}

func (reader *recordReader) offset() int64 { return reader.base + reader.csv.InputOffset() }

func (reader *recordReader) read() ([]string, error) {
	record, err := reader.csv.Read()
	if err != nil {
		return nil, err
	}
	reader.guard.mark = reader.csv.InputOffset()
	if reader.next == 0 && len(record) > 0 {
		record[0] = strings.TrimPrefix(record[0], "\ufeff")
	}
	reader.next++
	return record, nil
}

func (reader *recordReader) skipTo(record int) error {
	for reader.next < record {
		if _, err := reader.read(); err != nil {
			return err
		}
	}
	return nil
}

// recordGuard fails a read that runs more than maxRecordBytes past the end
// of the last whole record.
type recordGuard struct {
	reader io.Reader
	read   int64
	mark   int64
}

func (guard *recordGuard) Read(buffer []byte) (int, error) {
	if guard.read-guard.mark > maxRecordBytes {
		return 0, errRecordTooLong
	}
	count, err := guard.reader.Read(buffer)
	guard.read += int64(count)
	return count, err
}
