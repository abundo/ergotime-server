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
		1: {ID: 1, CustomerID: 7, Name: "Alpha", CostCenter: "CC1", Unit: "hours"},
		2: {ID: 2, CustomerID: 7, Name: "Beta", Unit: "hours"},
	}
	customers := map[int64]models.Customer{
		7: {ID: 7, Name: "Acme", Description: "Acme AB"},
	}

	got, total := buildMonthlyCustomers(reports, projects, customers, 0, time.UTC)
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

	if len(cu.Projects) != 2 {
		t.Fatalf("summary: %+v", cu.Projects)
	}
	if cu.Projects[0].Name != "Alpha" || cu.Projects[0].TotalHM != "01:00" || cu.Projects[0].CostCenter != "CC1" {
		t.Fatalf("alpha summary: %+v", cu.Projects[0])
	}
	if cu.Projects[1].Name != "Several projects" || cu.Projects[1].TotalHM != "01:30" {
		t.Fatalf("shared summary: %+v", cu.Projects[1])
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
	got, total := buildMonthlyCustomers(reports, projects, customers, 2, time.UTC)
	if total != time.Hour || len(got) != 1 || got[0].CustomerID != 2 {
		t.Fatalf("got %+v total=%v", got, total)
	}
}
