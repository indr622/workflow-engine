package domain

import (
	"fmt"
	"strings"
)

// notFoundError membawa reason stabil untuk ErrorInfo (ADR-0023).
type notFoundError struct{}

func (notFoundError) Error() string  { return "process definition not found" }
func (notFoundError) Reason() string { return "PROCESS_DEFINITION_NOT_FOUND" }

// ErrNotFound dikembalikan bila process definition tidak ada.
var ErrNotFound error = notFoundError{}

// Severity adalah tingkat keparahan violation.
type Severity string

// Nilai Severity.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Violation adalah satu masalah validasi, transport-agnostik.
type Violation struct {
	Code         string
	Severity     Severity
	Message      string
	ResourceName string
	ProcessID    string
	ElementID    string
	Line         int
	Column       int
}

func (v Violation) String() string {
	var b strings.Builder
	b.WriteString(string(v.Severity) + " " + v.Code)
	if v.ResourceName != "" {
		b.WriteString(" " + v.ResourceName)
	}
	if v.ElementID != "" {
		b.WriteString(" [" + v.ElementID + "]")
	}
	b.WriteString(": " + v.Message)
	return b.String()
}

// RejectedError berarti deployment ditolak; tidak ada yang tersimpan.
type RejectedError struct {
	Violations []Violation
}

// Reason memberi ErrorInfo.reason.
func (e *RejectedError) Reason() string { return "DEPLOYMENT_REJECTED" }

func (e *RejectedError) Error() string {
	n := 0
	for _, v := range e.Violations {
		if v.Severity == SeverityError {
			n++
		}
	}
	return fmt.Sprintf("deployment rejected with %d error(s)", n)
}

// Errors mengembalikan violation berseverity error saja.
func (e *RejectedError) Errors() []Violation {
	var out []Violation
	for _, v := range e.Violations {
		if v.Severity == SeverityError {
			out = append(out, v)
		}
	}
	return out
}
