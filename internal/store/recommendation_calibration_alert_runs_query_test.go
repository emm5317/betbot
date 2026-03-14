package store

import (
	"strings"
	"testing"
)

func TestInsertRecommendationCalibrationAlertRunQueryIsAppendOnly(t *testing.T) {
	query := strings.ToUpper(insertRecommendationCalibrationAlertRun)
	if !strings.Contains(query, "INSERT INTO RECOMMENDATION_CALIBRATION_ALERT_RUNS") {
		t.Fatalf("insert query missing target table: %s", insertRecommendationCalibrationAlertRun)
	}
	if strings.Contains(query, " UPDATE ") || strings.Contains(query, "\nUPDATE ") {
		t.Fatalf("insert query unexpectedly contains UPDATE: %s", insertRecommendationCalibrationAlertRun)
	}
	if strings.Contains(query, " DELETE ") || strings.Contains(query, "\nDELETE ") {
		t.Fatalf("insert query unexpectedly contains DELETE: %s", insertRecommendationCalibrationAlertRun)
	}
}

func TestListRecommendationCalibrationAlertRunsQueryHasDeterministicOrdering(t *testing.T) {
	if !strings.Contains(listRecommendationCalibrationAlertRuns, "ORDER BY created_at DESC, id DESC") {
		t.Fatalf("history query missing deterministic ordering: %s", listRecommendationCalibrationAlertRuns)
	}
}
