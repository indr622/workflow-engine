package feel

import "fmt"

// SyntaxError menandakan ekspresi yang rusak secara sintaks (juga tidak valid di FEEL penuh).
type SyntaxError struct {
	// Pos adalah offset byte (berbasis 0) di sumber ekspresi.
	Pos int
	Msg string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("feel: syntax error at offset %d: %s", e.Pos, e.Msg)
}

// UnsupportedError menandakan konstruksi FEEL yang sah tetapi di luar subset yang didukung.
type UnsupportedError struct {
	// Pos adalah offset byte (berbasis 0) di sumber ekspresi.
	Pos int
	Msg string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("feel: unsupported construct at offset %d: %s", e.Pos, e.Msg)
}

func syntaxErr(pos int, format string, args ...any) error {
	return &SyntaxError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

func unsupportedErr(pos int, format string, args ...any) error {
	return &UnsupportedError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}
