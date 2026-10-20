package launcher

import (
	"net"
	"testing"
)

func TestParseProcIP(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		"0100007F":                         "127.0.0.1",
		"00000000":                         "0.0.0.0",
		"0000000000000000FFFF00000100007F": "::ffff:127.0.0.1",
		"00000000000000000000000001000000": "::1",
	} {
		ip, err := parseProcIP(text)
		if err != nil || !ip.Equal(net.ParseIP(want)) {
			t.Errorf("parseProcIP(%q) = %v, %v; want %s", text, ip, err, want)
		}
	}
}
