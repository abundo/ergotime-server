package handlers

import (
	"testing"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
)

func TestBuildMonthlyCustomers_ProjectThenComment(t *testing.T) {
	start := time.Date(2026, 8, 3, 9, 15, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	next := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)
	nextStop := next.Add(time.Hour)

	reports := []models.Report{
		{
			ID:         10,
			CustomerID: 7,
			ProjectID:  1,
			Start:      start,
			Stop:       &stop,
			Comment:    "ignored when lines exist",
			Lines: []models.ReportLine{
				{ProjectID: 1, Comment: "fixed the valve"},
				{ProjectID: 2, Comment: "called the site"},
			},
		},
		{
			ID:         11,
			CustomerID: 7,
			ProjectID:  1,
			Start:      next,
			Stop:       &nextStop,
			Comment:    "solo note",
			Lines: []models.ReportLine{
				{ProjectID: 1, Comment: "solo note"},
			},
		},
	}
	projects := map[int64]models.Project{
		1: {ID: 1, CustomerID: 7, Name: "Alpha", CostCenter: "CC1", Unit: "hours", Active: true},
		2: {ID: 2, CustomerID: 7, Name: "Beta", Unit: "hours", Active: true},
	}
	customers := map[int64]models.Customer{
		7: {ID: 7, Name: "Acme", Description: "Acme AB"},
	}
	catalog := []models.Project{
		projects[1],
		projects[2],
		{ID: 3, CustomerID: 7, Name: "Gamma", CostCenter: "CC3", Unit: "hours", Active: true},
		{ID: 4, CustomerID: 7, Name: "Old", Unit: "hours", Active: false},
	}

	got, total := buildMonthlyCustomers(reports, projects, customers, catalog, 0, 0, time.UTC, nil)
	if total != 150*time.Minute || len(got) != 1 {
		t.Fatalf("customers=%d total=%v", len(got), total)
	}
	cu := got[0]
	if cu.Name != "Acme" || cu.TotalHM != "02:30" || len(cu.Days) != 2 {
		t.Fatalf("customer: %+v", cu)
	}

	day := cu.Days[0]
	if day.Date != "2026-08-03" || day.TotalHM != "01:30" || len(day.Entries) != 1 {
		t.Fatalf("day: %+v", day)
	}
	entry := day.Entries[0]
	if entry.ID != 10 || entry.Start != "09:15" || entry.Stop != "10:45" || entry.HM != "01:30" {
		t.Fatalf("entry: %+v", entry)
	}
	if len(entry.Notes) != 2 {
		t.Fatalf("notes: %+v", entry.Notes)
	}
	if entry.Notes[0].Project != "Alpha" || entry.Notes[0].Comment != "fixed the valve" {
		t.Fatalf("first note: %+v", entry.Notes[0])
	}
	if entry.Notes[1].Project != "Beta" || entry.Notes[1].Comment != "called the site" {
		t.Fatalf("second note: %+v", entry.Notes[1])
	}

	solo := cu.Days[1].Entries[0]
	if len(solo.Notes) != 1 || solo.Notes[0].Project != "Alpha" || solo.Notes[0].Comment != "solo note" {
		t.Fatalf("solo: %+v", solo)
	}
	if len(cu.Days[0].Projects) != 2 || cu.Days[0].Projects[0].Name != "Alpha" || cu.Days[0].Projects[0].TotalHM != "01:30" {
		t.Fatalf("shared day stays on anchor: %+v", cu.Days[0].Projects)
	}
	if cu.Days[0].Projects[1].Name != "Beta" || cu.Days[0].Projects[1].TotalHM != "00:00" {
		t.Fatalf("named project with no share: %+v", cu.Days[0].Projects[1])
	}
	if cu.Days[0].Entries[0].Notes[0].LengthHM != "01:30" || cu.Days[0].Entries[0].Notes[1].LengthHM != "00:00" {
		t.Fatalf("lengths: %+v", cu.Days[0].Entries[0].Notes)
	}
	if len(cu.Days[1].Projects) != 1 || cu.Days[1].Projects[0].Name != "Alpha" || cu.Days[1].Projects[0].TotalHM != "01:00" {
		t.Fatalf("solo day projects: %+v", cu.Days[1].Projects)
	}

	if len(cu.Projects) != 3 {
		t.Fatalf("summary: %+v", cu.Projects)
	}
	if cu.Projects[0].Name != "Alpha" || cu.Projects[0].TotalHM != "02:30" || cu.Projects[0].CostCenter != "CC1" {
		t.Fatalf("alpha summary: %+v", cu.Projects[0])
	}
	if cu.Projects[1].Name != "Beta" || cu.Projects[1].ProjectID != 2 || cu.Projects[1].TotalHM != "00:00" {
		t.Fatalf("beta summary: %+v", cu.Projects[1])
	}
	if cu.Projects[2].Name != "Gamma" || cu.Projects[2].ProjectID != 3 || cu.Projects[2].TotalHM != "00:00" || cu.Projects[2].CostCenter != "CC3" {
		t.Fatalf("gamma summary: %+v", cu.Projects[2])
	}
	if cu.Invoiced {
		t.Fatal("expected not invoiced")
	}
}

func TestBuildMonthlyCustomers_EachReportListsTheDaysProjects(t *testing.T) {
	morningStart := time.Date(2026, 10, 9, 9, 16, 0, 0, time.UTC)
	morningStop := morningStart.Add(2*time.Hour + 52*time.Minute)
	afternoonStart := time.Date(2026, 10, 9, 12, 38, 0, 0, time.UTC)
	afternoonStop := afternoonStart.Add(3*time.Hour + 25*time.Minute)
	reports := []models.Report{
		{
			ID: 1, CustomerID: 13, ProjectID: 66,
			Start: morningStart, Stop: &morningStop,
			Lines: []models.ReportLine{{ProjectID: 66, Comment: "flytt av server"}},
		},
		{
			ID: 2, CustomerID: 13, ProjectID: 22,
			Start: afternoonStart, Stop: &afternoonStop,
			Lines: []models.ReportLine{{ProjectID: 22, Comment: ""}},
		},
	}
	projects := map[int64]models.Project{
		66: {ID: 66, CustomerID: 13, Name: "SYSADM", Unit: "hours", Active: true},
		22: {ID: 22, CustomerID: 13, Name: "Wholesale", Unit: "hours", Active: true},
	}
	customers := map[int64]models.Customer{13: {ID: 13, Name: "IT Norrbotten"}}

	got, _ := buildMonthlyCustomers(reports, projects, customers, []models.Project{projects[66], projects[22]}, 0, 0, time.UTC, nil)
	if len(got) != 1 || len(got[0].Days) != 1 || len(got[0].Days[0].Entries) != 2 {
		t.Fatalf("shape: %+v", got)
	}
	morning := got[0].Days[0].Entries[0].Notes
	afternoon := got[0].Days[0].Entries[1].Notes
	if len(morning) != 2 || len(afternoon) != 2 {
		t.Fatalf("morning=%+v afternoon=%+v", morning, afternoon)
	}
	if morning[0].Project != "SYSADM" || morning[0].Comment != "flytt av server" || morning[0].LengthHM != "02:52" {
		t.Fatalf("morning sysadm: %+v", morning[0])
	}
	if morning[1].Project != "Wholesale" || morning[1].Comment != "" || morning[1].LengthHM != "" {
		t.Fatalf("morning wholesale: %+v", morning[1])
	}
	if afternoon[0].Project != "SYSADM" || afternoon[0].Comment != "" || afternoon[0].LengthHM != "" {
		t.Fatalf("afternoon sysadm: %+v", afternoon[0])
	}
	if afternoon[1].Project != "Wholesale" || afternoon[1].Comment != "" || afternoon[1].LengthHM != "03:25" {
		t.Fatalf("afternoon wholesale: %+v", afternoon[1])
	}
}

func TestDayWindowInMonth(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	start, end, err := dayWindowInMonth("2026-08-03", "2026-08", loc)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, loc)) || !end.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("window %v .. %v", start, end)
	}
	if _, _, err := dayWindowInMonth("2026-09-01", "2026-08", loc); err == nil {
		t.Fatal("expected date outside the month to fail")
	}
	if _, _, err := dayWindowInMonth("", "2026-08", loc); err == nil {
		t.Fatal("expected missing date to fail")
	}
}

func TestBuildMonthlyCustomers_FiltersCustomer(t *testing.T) {
	start := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	reports := []models.Report{
		{ID: 1, CustomerID: 1, ProjectID: 1, Start: start, Stop: &stop},
		{ID: 2, CustomerID: 2, ProjectID: 2, Start: start, Stop: &stop},
	}
	projects := map[int64]models.Project{
		1: {ID: 1, CustomerID: 1, Name: "A"},
		2: {ID: 2, CustomerID: 2, Name: "B"},
	}
	customers := map[int64]models.Customer{
		1: {ID: 1, Name: "One"},
		2: {ID: 2, Name: "Two"},
	}
	got, total := buildMonthlyCustomers(reports, projects, customers, nil, 2, 0, time.UTC, nil)
	if total != time.Hour || len(got) != 1 || got[0].CustomerID != 2 {
		t.Fatalf("got %+v total=%v", got, total)
	}
}
