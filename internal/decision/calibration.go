package decision

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	DefaultCalibrationBucketCount = 10
	MaxCalibrationBucketCount     = 20
)

type CalibrationOptions struct {
	BucketCount int
}

type CalibrationInputRow struct {
	RowID                  int64
	RankScore              float64
	ExpectedWinProbability float64
	Outcome                string
	CLVDelta               *float64
}

type CalibrationSummary struct {
	TotalRows              int     `json:"total_rows"`
	SettledRows            int     `json:"settled_rows"`
	ExcludedRows           int     `json:"excluded_rows"`
	OverallObservedWinRate float64 `json:"overall_observed_win_rate"`
	OverallExpectedWinRate float64 `json:"overall_expected_win_rate"`
	OverallBrier           float64 `json:"overall_brier"`
	OverallECE             float64 `json:"overall_ece"`
	AverageCLV             float64 `json:"avg_clv"`
}

type CalibrationBucket struct {
	BucketIndex     int      `json:"bucket_index"`
	RankMin         *float64 `json:"rank_min"`
	RankMax         *float64 `json:"rank_max"`
	Count           int      `json:"count"`
	SettledCount    int      `json:"settled_count"`
	ObservedWinRate float64  `json:"observed_win_rate"`
	ExpectedWinRate float64  `json:"expected_win_rate"`
	CalibrationGap  float64  `json:"calibration_gap"`
	Brier           float64  `json:"brier"`
	AverageCLV      float64  `json:"avg_clv"`
}

type CalibrationReport struct {
	BucketCount int                 `json:"bucket_count"`
	Summary     CalibrationSummary  `json:"summary"`
	Buckets     []CalibrationBucket `json:"buckets"`
}

type calibrationBucketAccumulator struct {
	CalibrationBucket
	observedWinSum float64
	expectedWinSum float64
	brierSum       float64
	clvSum         float64
	clvCount       int
}

type orderedCalibrationRow struct {
	CalibrationInputRow
	originalIndex int
}

func ComputeCalibrationReport(rows []CalibrationInputRow, opts CalibrationOptions) (CalibrationReport, error) {
	bucketCount, err := resolveCalibrationBucketCount(opts.BucketCount)
	if err != nil {
		return CalibrationReport{}, err
	}

	orderedRows := make([]orderedCalibrationRow, len(rows))
	for i := range rows {
		row := rows[i]
		if math.IsNaN(row.RankScore) || math.IsInf(row.RankScore, 0) {
			return CalibrationReport{}, fmt.Errorf("invalid rank score at row %d", i)
		}
		orderedRows[i] = orderedCalibrationRow{
			CalibrationInputRow: row,
			originalIndex:       i,
		}
	}

	sort.SliceStable(orderedRows, func(i, j int) bool {
		left := orderedRows[i]
		right := orderedRows[j]
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if left.RowID != right.RowID {
			return left.RowID < right.RowID
		}
		return left.originalIndex < right.originalIndex
	})

	accumulators := make([]calibrationBucketAccumulator, bucketCount)
	for i := range accumulators {
		accumulators[i].BucketIndex = i
	}

	var (
		totalObservedWinSum float64
		totalExpectedWinSum float64
		totalBrierSum       float64
		totalCLVSum         float64
		totalCLVCount       int
		settledRows         int
	)

	for i := range orderedRows {
		row := orderedRows[i]
		bucketIndex := (i * bucketCount) / len(orderedRows)
		bucket := &accumulators[bucketIndex]
		bucket.Count++
		updateBucketRankRange(bucket, row.RankScore)

		outcomeValue, include := calibrationOutcomeToBinary(row.Outcome)
		if !include {
			continue
		}
		if err := validateProbability(row.ExpectedWinProbability, "expected win probability"); err != nil {
			continue
		}

		brier := math.Pow(row.ExpectedWinProbability-outcomeValue, 2)
		bucket.SettledCount++
		bucket.observedWinSum += outcomeValue
		bucket.expectedWinSum += row.ExpectedWinProbability
		bucket.brierSum += brier
		if row.CLVDelta != nil {
			bucket.clvSum += *row.CLVDelta
			bucket.clvCount++
		}

		settledRows++
		totalObservedWinSum += outcomeValue
		totalExpectedWinSum += row.ExpectedWinProbability
		totalBrierSum += brier
		if row.CLVDelta != nil {
			totalCLVSum += *row.CLVDelta
			totalCLVCount++
		}
	}

	buckets := make([]CalibrationBucket, bucketCount)
	for i := range accumulators {
		bucket := &accumulators[i]
		if bucket.SettledCount > 0 {
			denominator := float64(bucket.SettledCount)
			bucket.ObservedWinRate = bucket.observedWinSum / denominator
			bucket.ExpectedWinRate = bucket.expectedWinSum / denominator
			bucket.CalibrationGap = bucket.ObservedWinRate - bucket.ExpectedWinRate
			bucket.Brier = bucket.brierSum / denominator
		}
		if bucket.clvCount > 0 {
			bucket.AverageCLV = bucket.clvSum / float64(bucket.clvCount)
		}
		buckets[i] = bucket.CalibrationBucket
	}

	summary := CalibrationSummary{
		TotalRows:    len(rows),
		SettledRows:  settledRows,
		ExcludedRows: len(rows) - settledRows,
	}

	if settledRows > 0 {
		denominator := float64(settledRows)
		summary.OverallObservedWinRate = totalObservedWinSum / denominator
		summary.OverallExpectedWinRate = totalExpectedWinSum / denominator
		summary.OverallBrier = totalBrierSum / denominator

		var ece float64
		for i := range buckets {
			bucket := buckets[i]
			if bucket.SettledCount == 0 {
				continue
			}
			weight := float64(bucket.SettledCount) / denominator
			ece += math.Abs(bucket.CalibrationGap) * weight
		}
		summary.OverallECE = ece
	}
	if totalCLVCount > 0 {
		summary.AverageCLV = totalCLVSum / float64(totalCLVCount)
	}

	return CalibrationReport{
		BucketCount: bucketCount,
		Summary:     summary,
		Buckets:     buckets,
	}, nil
}

func resolveCalibrationBucketCount(requested int) (int, error) {
	if requested == 0 {
		return DefaultCalibrationBucketCount, nil
	}
	if requested < 1 || requested > MaxCalibrationBucketCount {
		return 0, fmt.Errorf("invalid bucket_count %d; expected integer in [1,%d]", requested, MaxCalibrationBucketCount)
	}
	return requested, nil
}

func calibrationOutcomeToBinary(outcome string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case RecommendationResultWin:
		return 1, true
	case RecommendationResultLoss:
		return 0, true
	default:
		return 0, false
	}
}

func updateBucketRankRange(bucket *calibrationBucketAccumulator, score float64) {
	if bucket.RankMin == nil || score < *bucket.RankMin {
		bucket.RankMin = calibrationFloat64Ptr(score)
	}
	if bucket.RankMax == nil || score > *bucket.RankMax {
		bucket.RankMax = calibrationFloat64Ptr(score)
	}
}

func calibrationFloat64Ptr(value float64) *float64 {
	v := value
	return &v
}
