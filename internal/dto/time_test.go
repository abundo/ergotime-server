package dto

import (
	"testing"
	"time"
)

func TestParseTime_UTC(t *testing.T) {
	zoned, err := ParseTime("2026-11-09T23:08:05Z")
	if err != nil {
		t.Fatal(err)
	}
	naive, err := ParseTime("2026-11-09 23:08:05")
	if err != nil {
		t.Fatal(err)
	}
	offset, err := ParseTime("2026-11-10T00:08:05+01:00")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 9, 23, 8, 5, 0, time.UTC)
	for _, got := range []time.Time{zoned, naive, offset} {
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Fatalf("got %s want %s", got, want)
		}
	}
}
