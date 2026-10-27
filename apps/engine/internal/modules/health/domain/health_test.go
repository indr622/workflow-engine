package domain

import "testing"

func TestNewReport(t *testing.T) {
	tests := []struct {
		name    string
		results []DependencyResult
		want    Status
	}{
		{"no dependencies", nil, StatusServing},
		{"all serving", []DependencyResult{{Name: "db", Status: StatusServing}}, StatusServing},
		{
			"one not serving",
			[]DependencyResult{
				{Name: "db", Status: StatusServing},
				{Name: "cache", Status: StatusNotServing},
			},
			StatusNotServing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewReport("test", tt.results)
			if got.Status != tt.want {
				t.Fatalf("status = %s, want %s", got.Status, tt.want)
			}
		})
	}
}
