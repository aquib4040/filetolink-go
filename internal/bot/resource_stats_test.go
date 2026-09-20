package bot

import (
	"testing"
)

func TestGetAppResourceStats(t *testing.T) {
	stats := GetAppResourceStats()

	if stats.AppRSSBytes <= 0 {
		t.Errorf("expected positive AppRSSBytes, got %d", stats.AppRSSBytes)
	}
	if stats.DynoLimitBytes <= 0 {
		t.Errorf("expected positive DynoLimitBytes, got %d", stats.DynoLimitBytes)
	}
	if stats.RAMPercent < 0 || stats.RAMPercent > 100 {
		t.Errorf("expected RAMPercent between 0 and 100, got %f", stats.RAMPercent)
	}
	if stats.HeapSysBytes <= 0 {
		t.Errorf("expected positive HeapSysBytes, got %d", stats.HeapSysBytes)
	}
	if stats.CPUPercent < 0 {
		t.Errorf("expected non-negative CPUPercent, got %f", stats.CPUPercent)
	}
}
