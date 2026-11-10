package handlers

import (
	"bytes"
	"compress/zlib"
	"io"
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

func TestSummarizeReportsByProject_SharedIntervalOnce(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.Local)
	stop := start.Add(150 * time.Minute)
	reports := []models.Report{
		{ProjectID: 1, Start: start, Stop: &stop, Lines: []models.ReportLine{
			{ProjectID: 1, Comment: "a"},
			{ProjectID: 2, Comment: "b"},
		}},
	}
	projects := map[int64]models.Project{
		1: {ID: 1, Name: "Alpha", Unit: "hours"},
		2: {ID: 2, Name: "Beta", Unit: "hours"},
	}
	got := summarizeReportsByProject(reports, projects)
	if len(got) != 1 {
		t.Fatalf("expected only the shared row, got %+v", got)
	}
	if got[0].Name != "Several projects" || got[0].Duration != 150*time.Minute {
		t.Fatalf("got %+v", got[0])
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

func TestGroupReportsByDay_ProjectThenOwnComment(t *testing.T) {
	start := time.Date(2026, 8, 3, 9, 15, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	reports := []models.Report{
		{
			ProjectID: 1,
			Start:     start,
			Stop:      &stop,
			Comment:   "Alpha\nfixed the valve\n\nBeta\ncalled the site",
			Lines: []models.ReportLine{
				{ProjectID: 1, Comment: "fixed the valve"},
				{ProjectID: 2, Comment: "called the site"},
			},
		},
	}
	names := map[int64]string{1: "Alpha", 2: "Beta"}
	days, total := groupReportsByDay(reports, names, time.UTC)
	if total != 90*time.Minute || len(days) != 1 || len(days[0].Entries) != 1 {
		t.Fatalf("days=%+v total=%v", days, total)
	}
	notes := days[0].Entries[0].Notes
	if len(notes) != 2 {
		t.Fatalf("notes: %+v", notes)
	}
	if notes[0].Project != "Alpha" || notes[0].Comment != "fixed the valve" {
		t.Fatalf("first note: %+v", notes[0])
	}
	if notes[1].Project != "Beta" || notes[1].Comment != "called the site" {
		t.Fatalf("second note: %+v", notes[1])
	}
	if days[0].Entries[0].Start != "09:15" || days[0].Entries[0].Length != "01:30" {
		t.Fatalf("time: %+v", days[0].Entries[0])
	}
}

func TestGroupReportsByDay_DisplayZone(t *testing.T) {
	start := time.Date(2026, 11, 9, 23, 8, 5, 0, time.UTC)
	stop := start.Add(6 * time.Second)
	reports := []models.Report{{ID: 1, ProjectID: 1, Start: start, Stop: &stop}}
	names := map[int64]string{1: "Wholesale"}

	utcDays, _ := groupReportsByDay(reports, names, time.UTC)
	if utcDays[0].Date != "2026-11-09" || utcDays[0].Entries[0].Start != "23:08" {
		t.Fatalf("utc day: %+v", utcDays[0])
	}

	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	localDays, _ := groupReportsByDay(reports, names, loc)
	if localDays[0].Date != "2026-11-10" || localDays[0].Entries[0].Start != "00:08" {
		t.Fatalf("stockholm day: %+v", localDays[0])
	}
}

func TestParseMonthParam_UsesViewerZone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	_, start, end, err := parseMonthParam("2026-11", loc)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equal(time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("start %s", start.UTC())
	}
	if !end.Equal(time.Date(2026, 11, 30, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("end %s", end.UTC())
	}
	_, utcStart, _, err := parseMonthParam("2026-11", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if !utcStart.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("utc start %s", utcStart.UTC())
	}
}

func TestProjectSummariesForCustomer_IncludesActiveWithoutTime(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	reports := []models.Report{{
		ProjectID: 1,
		Start:     start,
		Stop:      &stop,
		Lines: []models.ReportLine{
			{ProjectID: 1, Comment: "a"},
			{ProjectID: 2, Comment: "b"},
		},
	}}
	projects := map[int64]models.Project{
		1: {ID: 1, CustomerID: 7, Name: "Alpha", Unit: "hours", Active: true},
		2: {ID: 2, CustomerID: 7, Name: "Beta", Unit: "hours", Active: true},
	}
	catalog := []models.Project{
		projects[1],
		projects[2],
		{ID: 3, CustomerID: 7, Name: "Gamma", Unit: "hours", Active: true},
		{ID: 4, CustomerID: 7, Name: "Old", Active: false},
	}
	got := projectSummariesForCustomer(reports, projects, catalog)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "Alpha" || got[0].Duration != 0 {
		t.Fatalf("alpha: %+v", got[0])
	}
	if got[1].Name != "Beta" || got[1].Duration != 0 {
		t.Fatalf("beta: %+v", got[1])
	}
	if got[2].Name != "Gamma" || got[2].Duration != 0 {
		t.Fatalf("gamma: %+v", got[2])
	}
}

func TestGenerateTimesheetPDF_ProjectColumn(t *testing.T) {
	days := []timesheetDay{{
		Date: "2026-08-03",
		Entries: []timesheetDayEntry{{
			Start:  "09:15",
			Stop:   "10:45",
			Length: "01:30",
			Notes: []timesheetNote{
				{Project: "Alpha", Comment: "fixed the valve"},
				{Project: "Beta", Comment: "called the site"},
			},
		}},
		Total: 90 * time.Minute,
	}}
	pdfBytes, err := generateTimesheetPDF("Acme", "", "August 2026", days, 90*time.Minute, []timesheetProjectSummary{
		{Name: "Alpha", Unit: "hours", Duration: time.Hour},
		{Name: "Gamma", Unit: "hours", Duration: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain := pdfStreams(t, pdfBytes)
	for _, s := range []string{"Project", "Comment", "Alpha", "fixed the valve", "Beta", "called the site", "09:15", "01:30", "Gamma", "00:00"} {
		if !bytes.Contains(plain, []byte(s)) {
			t.Errorf("pdf missing %q", s)
		}
	}
	// Project column is written before the comment column in the day table header.
	proj := bytes.Index(plain, []byte("(Project)"))
	comment := bytes.Index(plain, []byte("(Comment)"))
	if proj < 0 || comment < 0 || proj > comment {
		t.Fatalf("header order project=%d comment=%d", proj, comment)
	}
}

func pdfStreams(t *testing.T, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	rest := b
	for {
		i := bytes.Index(rest, []byte("stream\n"))
		if i < 0 {
			break
		}
		rest = rest[i+len("stream\n"):]
		j := bytes.Index(rest, []byte("\nendstream"))
		if j < 0 {
			break
		}
		r, err := zlib.NewReader(bytes.NewReader(rest[:j]))
		rest = rest[j+len("\nendstream"):]
		if err != nil {
			continue
		}
		decoded, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out.Write(decoded)
	}
	if out.Len() == 0 {
		t.Fatal("no pdf streams")
	}
	return out.Bytes()
}

func ptrTime(t time.Time) *time.Time { return &t }
