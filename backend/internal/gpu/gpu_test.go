package gpu

import "testing"

func TestParseRange(t *testing.T) {
	deck := `OD_SCLK:
0:        200Mhz
1:        1600Mhz
OD_RANGE:
SCLK:     200Mhz       1600Mhz
CCLK: 1400Mhz 3500Mhz
`
	lo, hi := parseRange(deck)
	if lo != 200 || hi != 1600 {
		t.Fatalf("got %d %d", lo, hi)
	}

	if _, hi := parseRange("OD_SCLK:\n0: 200Mhz\n"); hi != 0 {
		t.Fatalf("no range should give 0, got %d", hi)
	}
}
