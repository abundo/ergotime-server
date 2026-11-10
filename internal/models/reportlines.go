package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoReportLine means a report was saved with no usable project.
var ErrNoReportLine = errors.New("at least one project is required")

// ErrUnknownProject means a line names a project that was not loaded.
var ErrUnknownProject = errors.New("project not found")

// ErrMixedCustomer means lines belong to more than one customer.
var ErrMixedCustomer = errors.New("report lines must belong to one customer")

// LineInput is one project/comment row from a client.
type LineInput struct {
	ProjectID int64
	Comment   string
}

// NormalizeReportLines turns the comment table into stored lines.
// A row with a non-empty comment is kept. When every comment is empty, only
// anchorHint is kept (the preselected project, otherwise the customer's
// default, otherwise the first row). Several kept rows share one interval.
// anchorHint is the project the user preselected.
func NormalizeReportLines(projects []Project, inputs []LineInput, anchorHint int64) (customerID, anchor int64, comment string, lines []ReportLine, err error) {
	if len(inputs) == 0 {
		return 0, 0, "", nil, ErrNoReportLine
	}
	byID := make(map[int64]Project, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}

	type row struct {
		project Project
		comment string
	}
	var rows []row
	seen := map[int64]bool{}
	for _, in := range inputs {
		if in.ProjectID <= 0 || seen[in.ProjectID] {
			continue
		}
		p, ok := byID[in.ProjectID]
		if !ok {
			return 0, 0, "", nil, ErrUnknownProject
		}
		if customerID == 0 {
			customerID = p.CustomerID
		} else if p.CustomerID != customerID {
			return 0, 0, "", nil, ErrMixedCustomer
		}
		seen[in.ProjectID] = true
		rows = append(rows, row{project: p, comment: in.Comment})
	}
	if len(rows) == 0 {
		return 0, 0, "", nil, ErrNoReportLine
	}

	var kept []row
	for _, r := range rows {
		if strings.TrimSpace(r.comment) != "" {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		pick := rows[0].project.ID
		if anchorHint > 0 && seen[anchorHint] {
			pick = anchorHint
		} else {
			for _, r := range rows {
				if r.project.IsDefault {
					pick = r.project.ID
					break
				}
			}
		}
		for _, r := range rows {
			if r.project.ID == pick {
				kept = []row{{project: r.project, comment: ""}}
				break
			}
		}
	}

	lines = make([]ReportLine, len(kept))
	for i, r := range kept {
		lines[i] = ReportLine{
			ProjectID: r.project.ID,
			Comment:   r.comment,
			Position:  i,
		}
	}

	anchor = lines[0].ProjectID
	if len(lines) == 1 {
		comment = lines[0].Comment
		return customerID, anchor, comment, lines, nil
	}
	if anchorHint > 0 {
		for _, ln := range lines {
			if ln.ProjectID == anchorHint {
				anchor = anchorHint
				break
			}
		}
	}
	names := map[int64]string{}
	for _, r := range kept {
		names[r.project.ID] = r.project.Name
	}
	comment = FormatLineComments(lines, names)
	return customerID, anchor, comment, lines, nil
}

// FormatLineComments renders several project notes as the report comment.
// A single line is returned unchanged, without the project name.
func FormatLineComments(lines []ReportLine, names map[int64]string) string {
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return lines[0].Comment
	}
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteString("\n\n")
		}
		name := strings.TrimSpace(names[ln.ProjectID])
		if name == "" {
			name = fmt.Sprintf("Project #%d", ln.ProjectID)
		}
		b.WriteString(name)
		if text := strings.TrimSpace(ln.Comment); text != "" {
			b.WriteString("\n")
			b.WriteString(ln.Comment)
		}
	}
	return b.String()
}

// DisplayComment is the timesheet comment. One project keeps its own text.
// Several projects are labeled so the shared interval is readable.
func DisplayComment(r Report, names map[int64]string) string {
	if len(r.Lines) <= 1 {
		if len(r.Lines) == 1 {
			return r.Lines[0].Comment
		}
		return r.Comment
	}
	return FormatLineComments(r.Lines, names)
}

// AttributedProject reports where a single-project interval belongs.
// Shared is true when several lines share the interval and it must not be
// added to any one project.
func AttributedProject(r Report) (projectID int64, shared bool) {
	if len(r.Lines) > 1 {
		return 0, true
	}
	if len(r.Lines) == 1 {
		return r.Lines[0].ProjectID, false
	}
	return r.ProjectID, false
}
