package tableview

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// scores has a numeric column with a blank and a non-number in it, a text
// column with repeated values, and a short row.
const scores = `id,score,text
a,0.9,meeting room
b,0.75,Meeting notes
c,,thanks
d,n/a,meeting
e,10,meeting
f,0.80
`

func openScores(t *testing.T) *Table {
	t.Helper()
	table, err := OpenFile(fileTarget(t, "scores.csv", scores), Format{Separator: ',', Header: true})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func ids(page Page) string {
	var result []string
	for _, row := range page.Rows {
		result = append(result, row.Cells[0])
	}
	return strings.Join(result, "")
}

func TestFiltersSelectRows(t *testing.T) {
	table := openScores(t)
	for _, test := range []struct {
		name    string
		filters []Condition
		want    string
	}{
		{"numbers compare by value", []Condition{{Column: 1, Op: OpGreaterEqual, Value: "0.8"}}, "aef"},
		{"orderings skip blanks and non-numbers", []Condition{{Column: 1, Op: OpLess, Value: "100"}}, "abef"},
		{"equality compares numbers as numbers", []Condition{{Column: 1, Op: OpEqual, Value: "0.8"}}, "f"},
		{"equality compares text otherwise", []Condition{{Column: 1, Op: OpEqual, Value: "n/a"}}, "d"},
		{"not equal keeps blanks", []Condition{{Column: 1, Op: OpNotEqual, Value: "0.9"}}, "bcdef"},
		{"contains is case-sensitive", []Condition{{Column: 2, Op: OpContains, Value: "meeting"}}, "ade"},
		{"does not contain", []Condition{{Column: 2, Op: OpNotContains, Value: "meeting"}}, "bcf"},
		{"a short row's missing cell is empty", []Condition{{Column: 2, Op: OpEmpty}}, "f"},
		{"not empty", []Condition{{Column: 1, Op: OpNotEmpty}}, "abdef"},
		{"every condition must match", []Condition{{Column: 2, Op: OpContains, Value: "meeting"}, {Column: 1, Op: OpGreater, Value: "1"}}, "e"},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, err := table.Page(Query{Filters: test.filters})
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(page); got != test.want || page.Matched != len(test.want) || page.Total != 6 {
				t.Fatalf("rows %q matched %d of %d, want %q", got, page.Matched, page.Total, test.want)
			}
		})
	}
}

func TestFilteredPagesKeepFileNumbers(t *testing.T) {
	table := openScores(t)
	page, err := table.Page(Query{Filters: []Condition{{Column: 2, Op: OpContains, Value: "meeting"}}, Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ids(page) != "d" || page.Rows[0].Number != 4 || page.Matched != 3 || page.Offset != 1 {
		t.Fatalf("page = %+v", page)
	}
}

func TestSortOrdersNumbersThenText(t *testing.T) {
	table := openScores(t)
	for _, test := range []struct {
		sort Sort
		want string
	}{
		// Numbers by value, then non-numbers by text with blanks first;
		// descending keeps numbers first.
		{Sort{Column: 1}, "bfaecd"},
		{Sort{Column: 1, Descending: true}, "eafbdc"},
		{Sort{Column: 2}, "fbdeac"},
	} {
		sort := test.sort
		page, err := table.Page(Query{Sort: &sort})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(page); got != test.want {
			t.Errorf("sort %+v = %q, want %q", test.sort, got, test.want)
		}
	}
	// A sort and a filter combine, and a page of the sorted rows keeps the
	// file's numbers.
	page, err := table.Page(Query{Filters: []Condition{{Column: 1, Op: OpGreater, Value: "0"}}, Sort: &Sort{Column: 1, Descending: true}, Offset: 1, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if ids(page) != "af" || page.Rows[0].Number != 1 || page.Rows[1].Number != 6 || page.Matched != 4 {
		t.Fatalf("sorted page = %+v", page)
	}
}

func TestBadFiltersAreRefused(t *testing.T) {
	table := openScores(t)
	for _, filters := range [][]Condition{
		{{Column: 3, Op: OpEqual, Value: "x"}},
		{{Column: -1, Op: OpEqual, Value: "x"}},
		{{Column: 1, Op: "~", Value: "x"}},
		{{Column: 1, Op: OpGreater, Value: "high"}},
	} {
		if _, err := table.Page(Query{Filters: filters}); err == nil {
			t.Errorf("filters %+v accepted", filters)
		}
		if _, err := table.Describe(filters); err == nil {
			t.Errorf("describe with %+v accepted", filters)
		}
	}
	if _, err := table.Page(Query{Sort: &Sort{Column: 9}}); err == nil {
		t.Error("sort by a missing column accepted")
	}
	if _, err := ParseConditions(`{"column":1}`); err == nil {
		t.Error("a filter object instead of a list accepted")
	}
	parsed, err := ParseConditions(`[{"column":1,"op":">","value":"2"}]`)
	if err != nil || !reflect.DeepEqual(parsed, []Condition{{Column: 1, Op: OpGreater, Value: "2"}}) {
		t.Errorf("parsed %+v, %v", parsed, err)
	}
}

func TestDescribeSummarizesColumns(t *testing.T) {
	table := openScores(t)
	description, err := table.Describe(nil)
	if err != nil {
		t.Fatal(err)
	}
	if description.Matched != 6 || description.Total != 6 || len(description.Columns) != 3 {
		t.Fatalf("description = %+v", description)
	}
	score := description.Columns[1]
	// Four numbers (0.9, 0.75, 10, 0.80) of five non-empty cells.
	if score.Count != 5 || score.Numeric != 4 || score.Unique != 5 || *score.Min != 0.75 || *score.Max != 10 || math.Abs(*score.Sum-12.45) > 1e-9 || math.Abs(*score.Mean-3.1125) > 1e-9 {
		t.Fatalf("score = %s", describeJSON(score))
	}
	// pandas' sample standard deviation of the same values.
	if math.Abs(*score.Std-4.59210) > 1e-4 {
		t.Fatalf("std = %v", *score.Std)
	}
	text := description.Columns[2]
	if text.Count != 5 || text.Unique != 4 || text.Numeric != 0 || text.Mean != nil || text.Sum != nil {
		t.Fatalf("text = %s", describeJSON(text))
	}

	// Filters choose the rows described.
	description, err = table.Describe([]Condition{{Column: 2, Op: OpContains, Value: "meeting"}})
	if err != nil {
		t.Fatal(err)
	}
	if description.Matched != 3 || description.Columns[1].Numeric != 2 || *description.Columns[1].Sum != 10.9 || description.Columns[1].Std == nil {
		t.Fatalf("filtered = %s", describeJSON(description))
	}
	// One number has no spread.
	description, err = table.Describe([]Condition{{Column: 0, Op: OpEqual, Value: "e"}})
	if err != nil {
		t.Fatal(err)
	}
	if description.Columns[1].Std != nil || *description.Columns[1].Mean != 10 {
		t.Fatalf("one row = %s", describeJSON(description))
	}
}

func TestDescribeCountsUniqueValuesUpToALimit(t *testing.T) {
	var content strings.Builder
	content.WriteString("n,big\n")
	for n := range uniqueLimit + 5 {
		content.WriteString(strconv.Itoa(n) + ",1e308\n")
	}
	table, err := OpenFile(fileTarget(t, "many.csv", content.String()), Format{Separator: ',', Header: true})
	if err != nil {
		t.Fatal(err)
	}
	description, err := table.Describe(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := description.Columns[0]; n.Unique != uniqueLimit || !n.UniqueOver || n.Count != uniqueLimit+5 {
		t.Fatalf("n = %s", describeJSON(n))
	}
	// A sum past float64's range is left out; the description still
	// encodes as JSON.
	if big := description.Columns[1]; big.Sum != nil || big.Max == nil || *big.Max != 1e308 {
		t.Fatalf("big = %s", describeJSON(big))
	}
	if _, err := json.Marshal(description); err != nil {
		t.Fatal(err)
	}
}

func describeJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
