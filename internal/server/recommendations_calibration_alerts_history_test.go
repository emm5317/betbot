package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestHandleRecommendationsCalibrationAlertsHistoryRejectsInvalidDateRange(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts/history?date_from=2026-03-14&date_to=2026-03-01")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration/alerts/history invalid date range status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "invalid date range")
}

func TestHandleRecommendationsCalibrationAlertsHistoryReturnsRows(t *testing.T) {
	queries := &fakeReadQueries{
		listCalibrationAlertRunsRows: []store.RecommendationCalibrationAlertRun{
			{
				ID:                          12,
				Sport:                       "MLB",
				RequestHash:                 "req-2",
				RunGroupHash:                "grp-2",
				Mode:                        decision.CalibrationAlertModeRolling,
				StepIndex:                   1,
				StepCount:                   2,
				WindowDays:                  int32Ptr(7),
				CurrentFrom:                 pgtype.Date{Time: time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC), Valid: true},
				CurrentTo:                   pgtype.Date{Time: time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC), Valid: true},
				BaselineFrom:                pgtype.Date{Time: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				BaselineTo:                  pgtype.Date{Time: time.Date(2026, time.March, 7, 0, 0, 0, 0, time.UTC), Valid: true},
				BucketCount:                 10,
				RowLimit:                    500,
				MinSettledOverall:           100,
				MinSettledPerBucket:         20,
				WarnEceDelta:                0.02,
				CriticalEceDelta:            0.05,
				WarnBrierDelta:              0.01,
				CriticalBrierDelta:          0.02,
				AlertLevel:                  "warn",
				Reasons:                     json.RawMessage(`["ece delta 0.022000 exceeded warn threshold 0.020000"]`),
				CurrentOverallEce:           0.122,
				BaselineOverallEce:          0.10,
				EceDelta:                    0.022,
				CurrentOverallBrier:         0.24,
				BaselineOverallBrier:        0.23,
				BrierDelta:                  0.01,
				CurrentSettledRows:          420,
				BaselineSettledRows:         430,
				InsufficientOverallWindows:  0,
				CurrentInsufficientBuckets:  0,
				BaselineInsufficientBuckets: 0,
				Payload:                     []byte(`{"mode":"rolling"}`),
				CreatedAt:                   store.Timestamptz(time.Date(2026, time.March, 14, 13, 0, 0, 0, time.UTC)),
			},
			{
				ID:        11,
				Sport:     "MLB",
				Mode:      decision.CalibrationAlertModePointInTime,
				Reasons:   json.RawMessage(`["calibration drift within configured thresholds"]`),
				CreatedAt: store.Timestamptz(time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)),
			},
		},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts/history?sport=baseball_mlb&date_from=2026-03-01&date_to=2026-03-14&limit=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations/calibration/alerts/history status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Filters struct {
			Sport string `json:"sport"`
			Limit int    `json:"limit"`
		} `json:"filters"`
		Rows []struct {
			ID      int64    `json:"id"`
			Mode    string   `json:"mode"`
			Reasons []string `json:"reasons"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &payload); err != nil {
		t.Fatalf("decode /recommendations/calibration/alerts/history: %v", err)
	}
	if payload.Filters.Sport != "baseball_mlb" {
		t.Fatalf("filters.sport = %q, want baseball_mlb", payload.Filters.Sport)
	}
	if payload.Filters.Limit != 2 {
		t.Fatalf("filters.limit = %d, want 2", payload.Filters.Limit)
	}
	if len(payload.Rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(payload.Rows))
	}
	if payload.Rows[0].ID != 12 {
		t.Fatalf("rows[0].id = %d, want 12", payload.Rows[0].ID)
	}
	if payload.Rows[0].Mode != decision.CalibrationAlertModeRolling {
		t.Fatalf("rows[0].mode = %q, want rolling", payload.Rows[0].Mode)
	}
	if len(payload.Rows[0].Reasons) == 0 {
		t.Fatal("rows[0].reasons should not be empty")
	}
	if len(queries.listCalibrationAlertRunsCall) != 1 {
		t.Fatalf("ListRecommendationCalibrationAlertRuns call count = %d, want 1", len(queries.listCalibrationAlertRunsCall))
	}
}

func TestHandleRecommendationsCalibrationAlertsRollingReturnsTrendAndPersistsRuns(t *testing.T) {
	anchor := time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC)
	windows, err := decision.BuildRollingCalibrationDriftWindows(anchor, 2, 3)
	if err != nil {
		t.Fatalf("BuildRollingCalibrationDriftWindows() error = %v", err)
	}
	rangeRows := make(map[string][]store.ListRecommendationPerformanceSnapshotsRow, len(windows)*2)
	snapshotID := int64(5000)
	for i := range windows {
		currentFrom := pgtype.Date{Time: windows[i].CurrentFrom, Valid: true}
		currentTo := pgtype.Date{Time: windows[i].CurrentTo, Valid: true}
		baselineFrom := pgtype.Date{Time: windows[i].BaselineFrom, Valid: true}
		baselineTo := pgtype.Date{Time: windows[i].BaselineTo, Valid: true}

		rangeRows[recommendationPerformanceRangeKey(currentFrom, currentTo)] = []store.ListRecommendationPerformanceSnapshotsRow{
			buildSettledCalibrationRow(snapshotID, windows[i].CurrentTo, "home", 0.58, 0.60, 3, 1),
			buildSettledCalibrationRow(snapshotID+1, windows[i].CurrentTo, "away", 0.54, 0.47, 1, 2),
		}
		rangeRows[recommendationPerformanceRangeKey(baselineFrom, baselineTo)] = []store.ListRecommendationPerformanceSnapshotsRow{
			buildSettledCalibrationRow(snapshotID+2, windows[i].BaselineTo, "home", 0.56, 0.57, 2, 1),
			buildSettledCalibrationRow(snapshotID+3, windows[i].BaselineTo, "away", 0.52, 0.45, 1, 3),
		}
		snapshotID += 4
	}

	queries := &fakeReadQueries{listPerformanceRowsByRange: rangeRows}
	app := newTestServerApp(t, queries)

	path := "/recommendations/calibration/alerts?mode=rolling&sport=baseball_mlb&current_to=2026-03-14&window_days=2&steps=3&bucket_count=2&limit=2&min_settled_overall=1&min_settled_per_bucket=1"
	resp := doRequest(t, app.app, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations/calibration/alerts rolling status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Filters struct {
			Mode       string `json:"mode"`
			WindowDays int    `json:"window_days"`
			Steps      int    `json:"steps"`
			CurrentTo  string `json:"current_to"`
		} `json:"filters"`
		Trend []struct {
			WindowStart string `json:"window_start"`
			WindowEnd   string `json:"window_end"`
		} `json:"trend"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &payload); err != nil {
		t.Fatalf("decode rolling /recommendations/calibration/alerts: %v", err)
	}
	if payload.Filters.Mode != decision.CalibrationAlertModeRolling {
		t.Fatalf("filters.mode = %q, want rolling", payload.Filters.Mode)
	}
	if payload.Filters.WindowDays != 2 || payload.Filters.Steps != 3 {
		t.Fatalf("filters rolling params = %+v, want window_days=2 steps=3", payload.Filters)
	}
	if payload.Filters.CurrentTo != "2026-03-14" {
		t.Fatalf("filters.current_to = %q, want 2026-03-14", payload.Filters.CurrentTo)
	}
	if len(payload.Trend) != 3 {
		t.Fatalf("len(trend) = %d, want 3", len(payload.Trend))
	}
	for i := 1; i < len(payload.Trend); i++ {
		if payload.Trend[i-1].WindowEnd > payload.Trend[i].WindowEnd {
			t.Fatalf("trend ordering not deterministic: %s before %s", payload.Trend[i-1].WindowEnd, payload.Trend[i].WindowEnd)
		}
	}
	if len(queries.insertCalibrationAlertRunCalls) != 3 {
		t.Fatalf("InsertRecommendationCalibrationAlertRun call count = %d, want 3", len(queries.insertCalibrationAlertRunCalls))
	}
	for i := range queries.insertCalibrationAlertRunCalls {
		call := queries.insertCalibrationAlertRunCalls[i]
		if call.Mode != decision.CalibrationAlertModeRolling {
			t.Fatalf("insert call[%d].mode = %q, want rolling", i, call.Mode)
		}
		if call.StepIndex != int32(i) {
			t.Fatalf("insert call[%d].step_index = %d, want %d", i, call.StepIndex, i)
		}
		if call.StepCount != 3 {
			t.Fatalf("insert call[%d].step_count = %d, want 3", i, call.StepCount)
		}
	}
}

func buildSettledCalibrationRow(snapshotID int64, eventDate time.Time, recommendedSide string, marketProbability float64, closeProbability float64, homeScore int, awayScore int) store.ListRecommendationPerformanceSnapshotsRow {
	raw := fmt.Sprintf(`{"completed":true,"scores":[{"name":"Home %d","score":"%d"},{"name":"Away %d","score":"%d"}]}`, snapshotID, homeScore, snapshotID, awayScore)
	return store.ListRecommendationPerformanceSnapshotsRow{
		SnapshotID:        snapshotID,
		GeneratedAt:       store.Timestamptz(eventDate.Add(-2 * time.Hour)),
		Sport:             "MLB",
		GameID:            snapshotID,
		HomeTeam:          fmt.Sprintf("Home %d", snapshotID),
		AwayTeam:          fmt.Sprintf("Away %d", snapshotID),
		EventTime:         store.Timestamptz(eventDate.Add(4 * time.Hour)),
		EventDate:         pgtype.Date{Time: eventDate, Valid: true},
		MarketKey:         "h2h",
		RecommendedSide:   recommendedSide,
		BestBook:          "book-a",
		BestAmericanOdds:  101,
		MarketProbability: marketProbability,
		RankScore:         float64(snapshotID),
		CloseLineID:       snapshotID + 10000,
		CloseProbability:  closeProbability,
		CloseCapturedAt:   store.Timestamptz(eventDate.Add(6 * time.Hour)),
		CloseRawJson:      json.RawMessage(raw),
	}
}
