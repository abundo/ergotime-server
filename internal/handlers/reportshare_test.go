package handlers

import (
	"testing"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
)

func TestPlanDayShareMove_KeepsTheReports(t *testing.T) {
	morningStart := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	morningStop := morningStart.Add(2*time.Hour + 30*time.Minute)
	afternoonStart := time.Date(2026, 11, 2, 11, 30, 0, 0, time.UTC)
	afternoonStop := afternoonStart.Add(2 * time.Hour)
	reports := []models.Report{
		{
			ID: 1, UserID: 2, CustomerID: 7, ProjectID: 22,
			Start: morningStart, Stop: &morningStop,
			Comment: "reconfigured border router",
			Lines: []models.ReportLine{
				{ProjectID: 22, Comment: "reconfigured border router"},
				{ProjectID: 66, Comment: "upgraded server"},
			},
		},
		{
			ID: 2, UserID: 2, CustomerID: 7, ProjectID: 22,
			Start: afternoonStart, Stop: &afternoonStop,
			Lines: []models.ReportLine{{ProjectID: 22, Comment: "afternoon note"}},
		},
	}
	current := dayBilling(reports, nil)
	if current[22] != int64((4*time.Hour+30*time.Minute)/time.Second) || current[66] != 0 {
		t.Fatalf("default billing: %v", current)
	}
	got, err := planDayShareMove(current, 22, 66, int64((45*time.Minute)/time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got[22] != int64((3*time.Hour+45*time.Minute)/time.Second) || got[66] != int64((45*time.Minute)/time.Second) {
		t.Fatalf("shares: %v", got)
	}
	if !reports[0].Start.Equal(morningStart) || !reports[0].Stop.Equal(morningStop) || reports[0].Comment != "reconfigured border router" {
		t.Fatalf("morning report changed: %+v", reports[0])
	}
	if !reports[1].Stop.Equal(afternoonStop) {
		t.Fatalf("afternoon report changed: %+v", reports[1])
	}
}

func TestPlanDayShareMove_NotEnough(t *testing.T) {
	_, err := planDayShareMove(map[int64]int64{1: int64(time.Hour / time.Second)}, 1, 2, int64((2*time.Hour)/time.Second))
	if err == nil {
		t.Fatal("expected not enough time")
	}
}

func TestDayBilling_OneProjectCannotSplit(t *testing.T) {
	start := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	stop := start.Add(2*time.Hour + 30*time.Minute)
	reports := []models.Report{{
		ProjectID: 22, Start: start, Stop: &stop,
		Lines: []models.ReportLine{{ProjectID: 22, Comment: "worked on xxxx"}},
	}}
	if len(projectsNamedOnReports(reports)) != 1 {
		t.Fatal("expected a single project")
	}
	got := dayBilling(reports, nil)
	if got[22] != int64((2*time.Hour+30*time.Minute)/time.Second) {
		t.Fatalf("got %v", got)
	}
}
