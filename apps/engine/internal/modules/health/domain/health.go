// Package domain berisi model dan aturan bisnis kesehatan engine.
// Paket ini tidak boleh bergantung pada framework, driver, maupun lapisan lain.
package domain

// Status adalah status layanan sebuah komponen.
type Status string

const (
	// StatusServing berarti komponen berfungsi normal.
	StatusServing Status = "serving"
	// StatusNotServing berarti komponen tidak dapat melayani request.
	StatusNotServing Status = "not_serving"
)

// DependencyResult adalah hasil pemeriksaan satu dependensi.
type DependencyResult struct {
	Name    string
	Status  Status
	Message string
}

// Report adalah laporan kesehatan agregat engine.
type Report struct {
	Status       Status
	Dependencies []DependencyResult
	Version      string
}

// NewReport menyusun laporan dari hasil tiap dependensi.
// Aturan bisnis: engine hanya serving jika SEMUA dependensi serving.
func NewReport(version string, results []DependencyResult) Report {
	status := StatusServing
	for _, r := range results {
		if r.Status != StatusServing {
			status = StatusNotServing
			break
		}
	}
	return Report{Status: status, Dependencies: results, Version: version}
}

// IsServing mengembalikan true jika engine sehat.
func (r Report) IsServing() bool { return r.Status == StatusServing }
