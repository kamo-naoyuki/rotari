package projectrun

import (
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestParseLoadAverage(t *testing.T) {
	got := parseLoadAverage("0.52 1.25 2.00 3/512 12345\n")
	want := &model.LoadAverage{One: 0.52, Five: 1.25, Fifteen: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLoadAverage = %+v, want %+v", got, want)
	}
	for _, input := range []string{"", "0.52 1.25", "0.52 x 2.00"} {
		if got := parseLoadAverage(input); got != nil {
			t.Fatalf("parseLoadAverage(%q) = %+v, want nil", input, got)
		}
	}
}
