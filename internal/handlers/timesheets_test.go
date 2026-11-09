package handlers

import (
	"testing"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
)

func TestSummarizeReportsByProject(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.Local)

	stop := func(d time.Duration) *time.Time {
		t := start.Add(d)
		return &t
	}

	reports := []models.Report{
		{ProjectID: 1, Start: start, Stop: stop(2 * time.Hour)},
		{ProjectID: 1, Start: start.Add(3 * time.Hour), Stop: stop(3*time.Hour + 30*time.Minute)},
		{ProjectID: 2, Start: start, Stop: stop(1 * time.Hour)},
		{ProjectID: 3, Start: start, Stop: stop(4 * time.Hour)},
		{ProjectID: 1, Start: start, Stop: nil},
		{ProjectID: 2, Start: start, Stop: ptrTime(start.Add(-time.Hour))},
	}

	projects := map[int64]models.Project{
		1: {ID: 1, Name: "Alpha", Unit: "hours"},
		2: {ID: 2, Name: "Beta", Unit: "hours"},
		3: {ID: 3, Name: "Travel", Unit: "kilometers"},
	}

	got := summarizeReportsByProject(reports, projects)
	if len(got) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(got))
	}

	if got[0].Name != "Alpha" || got[1].Name != "Beta" || got[2].Name != "Travel" {
		t.Fatalf("unexpected order/names: %+v", []string{got[0].Name, got[1].Name, got[2].Name})
	}

	if got[0].Duration != 2*time.Hour+30*time.Minute || got[0].Unit != "hours" {
		t.Errorf("Alpha: duration=%v unit=%q", got[0].Duration, got[0].Unit)
	}
	if got[1].Duration != time.Hour || got[1].Unit != "hours" {
		t.Errorf("Beta: duration=%v unit=%q", got[1].Duration, got[1].Unit)
	}
	if got[2].Duration != 4*time.Hour || got[2].Unit != "kilometers" {
		t.Errorf("Travel: duration=%v unit=%q", got[2].Duration, got[2].Unit)
	}
}

func TestSummarizeReportsByProject_Empty(t *testing.T) {
	got := summarizeReportsByProject(nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestSummarizeReportsByProject_UnknownProject(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.Local)
	stop := start.Add(time.Hour)
	reports := []models.Report{
		{ProjectID: 99, Start: start, Stop: &stop},
	}
	got := summarizeReportsByProject(reports, map[int64]models.Project{})
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Name != "Project #99" {
		t.Errorf("fallback name: got %q", got[0].Name)
	}
	if got[0].Unit != "hours" {
		t.Errorf("default unit: got %q", got[0].Unit)
	}
}

func TestParseHMDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"2:30", 2*time.Hour + 30*time.Minute, true},
		{"02:05", 2*time.Hour + 5*time.Minute, true},
		{"0:15", 15 * time.Minute, true},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"", 0, false},
		{"abc", 0, false},
		{"1:60", 0, false},
	}
	for _, tc := range cases {
		got, err := parseHMDuration(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("parseHMDuration(%q): unexpected err %v", tc.in, err)
				continue
			}
			if got != tc.want {
				t.Errorf("parseHMDuration(%q) = %v, want %v", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Errorf("parseHMDuration(%q): expected error, got %v", tc.in, got)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
