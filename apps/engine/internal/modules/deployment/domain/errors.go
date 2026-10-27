package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound dikembalikan bila process definition tidak ada.
var ErrNotFound = errors.New("process definition not found")

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
