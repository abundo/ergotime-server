package models

import "testing"

func TestNormalizeReportLines_DefaultWhenEmpty(t *testing.T) {
	projects := []Project{
		{ID: 1, CustomerID: 7, Name: "Alpha", IsDefault: true},
		{ID: 2, CustomerID: 7, Name: "Beta"},
	}
	inputs := []LineInput{{ProjectID: 1, Comment: "  "}, {ProjectID: 2, Comment: ""}}
	cust, anchor, comment, lines, err := NormalizeReportLines(projects, inputs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cust != 7 || anchor != 1 || comment != "" || len(lines) != 1 || lines[0].ProjectID != 1 {
		t.Fatalf("got cust=%d anchor=%d comment=%q lines=%+v", cust, anchor, comment, lines)
	}
}

func TestNormalizeReportLines_SharedKeepsNamedRows(t *testing.T) {
	projects := []Project{
		{ID: 1, CustomerID: 7, Name: "Alpha", IsDefault: true},
		{ID: 2, CustomerID: 7, Name: "Beta"},
	}
	inputs := []LineInput{
		{ProjectID: 1, Comment: "plan"},
		{ProjectID: 2, Comment: "build"},
	}
	_, anchor, comment, lines, err := NormalizeReportLines(projects, inputs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || anchor != 1 {
		t.Fatalf("lines=%+v anchor=%d", lines, anchor)
	}
	if comment != "Alpha\nplan\n\nBeta\nbuild" {
		t.Fatalf("comment=%q", comment)
	}
	sharedID, shared := AttributedProject(Report{ProjectID: anchor, Lines: lines})
	if !shared || sharedID != 0 {
		t.Fatalf("shared=%v id=%d", shared, sharedID)
	}
}

func TestNormalizeReportLines_OnlyOneComment(t *testing.T) {
	projects := []Project{
		{ID: 1, CustomerID: 7, Name: "Alpha", IsDefault: true},
		{ID: 2, CustomerID: 7, Name: "Beta"},
	}
	_, anchor, comment, lines, err := NormalizeReportLines(projects, []LineInput{
		{ProjectID: 1, Comment: ""},
		{ProjectID: 2, Comment: "only beta"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || anchor != 2 || comment != "only beta" {
		t.Fatalf("anchor=%d comment=%q lines=%+v", anchor, comment, lines)
	}
}
