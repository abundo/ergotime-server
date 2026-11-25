package handlers

import (
	"testing"
	"time"
)

func TestValidateReportTimes(t *testing.T) {
	start := time.Date(2026, 11, 25, 12, 20, 0, 0, time.UTC)
	stop := start.Add(time.Minute)
	if err := validateReportTimes(start, &stop); err != nil {
		t.Fatal(err)
	}
	// An open report has no stop. Its start is still a whole minute.
	if err := validateReportTimes(start, nil); err != nil {
		t.Fatal(err)
	}

	same := start
	if err := validateReportTimes(start, &same); err != errReportTooShort {
		t.Fatalf("same minute: %v", err)
	}
	// 12:20:59 is 59 seconds and not a minute boundary.
	late := start.Add(59 * time.Second)
	if err := validateReportTimes(start, &late); err != errStopNotMinute {
		t.Fatalf(":59 stop: %v", err)
	}
	withSeconds := start.Add(time.Second)
	if err := validateReportTimes(withSeconds, &stop); err != errStartNotMinute {
		t.Fatalf("start seconds: %v", err)
	}
	stopSeconds := stop.Add(time.Second)
	if err := validateReportTimes(start, &stopSeconds); err != errStopNotMinute {
		t.Fatalf("stop seconds: %v", err)
	}
	// A fraction of a second is still off the boundary.
	frac := start.Add(time.Millisecond)
	if err := validateReportTimes(frac, &stop); err != errStartNotMinute {
		t.Fatalf("start fraction: %v", err)
	}
}
