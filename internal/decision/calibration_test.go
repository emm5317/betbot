package decision

import (
	"math"
	"testing"
)

func TestComputeCalibrationReportEmptyInput(t *testing.T) {
	report, err := ComputeCalibrationReport(nil, CalibrationOptions{})
	if err != nil {
		t.Fatalf("ComputeCalibrationReport() error = %v", err)
	}

	if report.BucketCount != DefaultCalibrationBucketCount {
		t.Fatalf("BucketCount = %d, want %d", report.BucketCount, DefaultCalibrationBucketCount)
	}
	if len(report.Buckets) != DefaultCalibrationBucketCount {
		t.Fatalf("len(Buckets) = %d, want %d", len(report.Buckets), DefaultCalibrationBucketCount)
	}
	if report.Summary.TotalRows != 0 || report.Summary.SettledRows != 0 || report.Summary.ExcludedRows != 0 {
		t.Fatalf("summary rows = %+v, want all zeros", report.Summary)
	}
	for i, bucket := range report.Buckets {
		if bucket.BucketIndex != i {
			t.Fatalf("bucket[%d].BucketIndex = %d, want %d", i, bucket.BucketIndex, i)
		}
		if bucket.Count != 0 || bucket.SettledCount != 0 {
			t.Fatalf("bucket[%d] counts = (%d,%d), want (0,0)", i, bucket.Count, bucket.SettledCount)
		}
		if bucket.RankMin != nil || bucket.RankMax != nil {
			t.Fatalf("bucket[%d] rank bounds expected nil, got min=%v max=%v", i, bucket.RankMin, bucket.RankMax)
		}
	}
}

func TestComputeCalibrationReportNoSettledRows(t *testing.T) {
	rows := []CalibrationInputRow{
		{RowID: 1, RankScore: 100, ExpectedWinProbability: 0.55, Outcome: RecommendationResultUnknown},
		{RowID: 2, RankScore: 90, ExpectedWinProbability: 0.53, Outcome: RecommendationResultPush},
		{RowID: 3, RankScore: 80, ExpectedWinProbability: 0.51, Outcome: "pending"},
	}

	report, err := ComputeCalibrationReport(rows, CalibrationOptions{BucketCount: 2})
	if err != nil {
		t.Fatalf("ComputeCalibrationReport() error = %v", err)
	}

	if report.Summary.TotalRows != 3 {
		t.Fatalf("TotalRows = %d, want 3", report.Summary.TotalRows)
	}
	if report.Summary.SettledRows != 0 {
		t.Fatalf("SettledRows = %d, want 0", report.Summary.SettledRows)
	}
	if report.Summary.ExcludedRows != 3 {
		t.Fatalf("ExcludedRows = %d, want 3", report.Summary.ExcludedRows)
	}
	if report.Buckets[0].Count != 2 || report.Buckets[1].Count != 1 {
		t.Fatalf("bucket counts = (%d,%d), want (2,1)", report.Buckets[0].Count, report.Buckets[1].Count)
	}
}

func TestComputeCalibrationReportTiesAroundBucketBoundaries(t *testing.T) {
	rows := []CalibrationInputRow{
		{RowID: 5, RankScore: 10, ExpectedWinProbability: 0.60, Outcome: RecommendationResultWin},
		{RowID: 2, RankScore: 10, ExpectedWinProbability: 0.60, Outcome: RecommendationResultLoss},
		{RowID: 4, RankScore: 10, ExpectedWinProbability: 0.60, Outcome: RecommendationResultWin},
		{RowID: 1, RankScore: 10, ExpectedWinProbability: 0.60, Outcome: RecommendationResultLoss},
		{RowID: 6, RankScore: 9, ExpectedWinProbability: 0.50, Outcome: RecommendationResultWin},
		{RowID: 3, RankScore: 8, ExpectedWinProbability: 0.40, Outcome: RecommendationResultLoss},
	}

	report, err := ComputeCalibrationReport(rows, CalibrationOptions{BucketCount: 3})
	if err != nil {
		t.Fatalf("ComputeCalibrationReport() error = %v", err)
	}

	if report.Summary.TotalRows != 6 || report.Summary.SettledRows != 6 || report.Summary.ExcludedRows != 0 {
		t.Fatalf("summary rows = %+v, want total=6 settled=6 excluded=0", report.Summary)
	}

	if got, want := report.Buckets[0].ObservedWinRate, 0.0; !nearlyEqual(got, want) {
		t.Fatalf("bucket[0].ObservedWinRate = %.6f, want %.6f", got, want)
	}
	if got, want := report.Buckets[1].ObservedWinRate, 1.0; !nearlyEqual(got, want) {
		t.Fatalf("bucket[1].ObservedWinRate = %.6f, want %.6f", got, want)
	}
	if got, want := report.Buckets[2].ObservedWinRate, 0.5; !nearlyEqual(got, want) {
		t.Fatalf("bucket[2].ObservedWinRate = %.6f, want %.6f", got, want)
	}

	if got, want := report.Summary.OverallECE, 0.35; !nearlyEqual(got, want) {
		t.Fatalf("OverallECE = %.6f, want %.6f", got, want)
	}
}

func TestComputeCalibrationReportDeterministicOrdering(t *testing.T) {
	rows := []CalibrationInputRow{
		{RowID: 9, RankScore: 105, ExpectedWinProbability: 0.62, Outcome: RecommendationResultWin},
		{RowID: 1, RankScore: 110, ExpectedWinProbability: 0.58, Outcome: RecommendationResultLoss},
		{RowID: 5, RankScore: 105, ExpectedWinProbability: 0.57, Outcome: RecommendationResultWin},
		{RowID: 3, RankScore: 98, ExpectedWinProbability: 0.52, Outcome: RecommendationResultUnknown},
		{RowID: 7, RankScore: 96, ExpectedWinProbability: 0.49, Outcome: RecommendationResultLoss},
	}
	reordered := []CalibrationInputRow{
		rows[4],
		rows[2],
		rows[0],
		rows[3],
		rows[1],
	}

	first, err := ComputeCalibrationReport(rows, CalibrationOptions{BucketCount: 4})
	if err != nil {
		t.Fatalf("first ComputeCalibrationReport() error = %v", err)
	}
	second, err := ComputeCalibrationReport(reordered, CalibrationOptions{BucketCount: 4})
	if err != nil {
		t.Fatalf("second ComputeCalibrationReport() error = %v", err)
	}

	if len(first.Buckets) != len(second.Buckets) {
		t.Fatalf("bucket lengths differ: %d vs %d", len(first.Buckets), len(second.Buckets))
	}
	for i := range first.Buckets {
		left := first.Buckets[i]
		right := second.Buckets[i]
		if left.BucketIndex != right.BucketIndex || left.Count != right.Count || left.SettledCount != right.SettledCount {
			t.Fatalf("bucket[%d] mismatch: left=%+v right=%+v", i, left, right)
		}
		if !equalNullableFloat(left.RankMin, right.RankMin) || !equalNullableFloat(left.RankMax, right.RankMax) {
			t.Fatalf("bucket[%d] rank bounds mismatch: left=(%v,%v) right=(%v,%v)", i, left.RankMin, left.RankMax, right.RankMin, right.RankMax)
		}
		if !nearlyEqual(left.ObservedWinRate, right.ObservedWinRate) ||
			!nearlyEqual(left.ExpectedWinRate, right.ExpectedWinRate) ||
			!nearlyEqual(left.CalibrationGap, right.CalibrationGap) ||
			!nearlyEqual(left.Brier, right.Brier) ||
			!nearlyEqual(left.AverageCLV, right.AverageCLV) {
			t.Fatalf("bucket[%d] metrics mismatch: left=%+v right=%+v", i, left, right)
		}
	}

	if !nearlyEqual(first.Summary.OverallECE, second.Summary.OverallECE) {
		t.Fatalf("OverallECE mismatch: %.6f vs %.6f", first.Summary.OverallECE, second.Summary.OverallECE)
	}
}

func nearlyEqual(left, right float64) bool {
	return math.Abs(left-right) < 1e-9
}

func equalNullableFloat(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return nearlyEqual(*left, *right)
}
