package moneypuck

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// AsPlayedRow is a parsed row from an NHL as-played CSV.
// Fields are nil when the CSV doesn't include them (e.g. 2024-25 has no odds).
type AsPlayedRow struct {
	GameDate       pgtype.Date
	CommenceTime   time.Time
	HomeTeam       string // Canonical abbreviation
	AwayTeam       string // Canonical abbreviation
	HomeScore      int
	AwayScore      int
	Status         string // Regulation, OT, SO, Scheduled
	HomeGoalie     string // Empty if not in CSV
	AwayGoalie     string // Empty if not in CSV
	HasOdds        bool
	HomeMoneyLine  int
	AwayMoneyLine  int
	HomeSpread     float64
	HomeSpreadLine int
	AwaySpreadLine int
	OverUnder      float64
	OverLine       int
	UnderLine      int
}

var asPlayedRequiredCols = []string{
	"Date", "Visitor", "Home", "Status",
}

// AsPlayedCSVReader reads NHL as-played CSVs (2024-25 scores-only or 2025-26 with odds).
type AsPlayedCSVReader struct {
	reader   *csv.Reader
	idx      columnIndex
	teamMap  TeamMap
	hasOdds  bool
	scoreCols [2]int // indices for the two "Score" columns (visitor, home)
}

// NewAsPlayedCSVReader creates a reader for as-played CSVs.
func NewAsPlayedCSVReader(r io.Reader, tm TeamMap) (*AsPlayedCSVReader, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := buildColumnIndex(headers)
	if err := idx.require(asPlayedRequiredCols...); err != nil {
		return nil, err
	}

	// Find the two "Score" columns by position (visitor score, then home score).
	var scoreCols [2]int
	found := 0
	for i, h := range headers {
		if strings.TrimSpace(h) == "Score" {
			if found < 2 {
				scoreCols[found] = i
			}
			found++
		}
	}
	if found < 2 {
		return nil, fmt.Errorf("expected 2 Score columns, found %d", found)
	}

	_, hasOdds := idx["Home ML"]

	return &AsPlayedCSVReader{
		reader:    cr,
		idx:       idx,
		teamMap:   tm,
		hasOdds:   hasOdds,
		scoreCols: scoreCols,
	}, nil
}

// Next reads the next completed game row. Skips "Scheduled" games. Returns nil, io.EOF when done.
func (r *AsPlayedCSVReader) Next() (*AsPlayedRow, error) {
	for {
		record, err := r.reader.Read()
		if err != nil {
			return nil, err
		}

		status := strings.TrimSpace(record[r.idx["Status"]])
		if status == "Scheduled" {
			continue
		}

		dateStr := r.idx.str(record, "Date")
		gameDate, err := parseISODate(dateStr)
		if err != nil {
			return nil, fmt.Errorf("row date %q: %w", dateStr, err)
		}

		visitorName := r.idx.str(record, "Visitor")
		away, err := r.teamMap.FromOddsAPIName(visitorName)
		if err != nil {
			return nil, fmt.Errorf("row visitor %q: %w", visitorName, err)
		}

		homeName := r.idx.str(record, "Home")
		home, err := r.teamMap.FromOddsAPIName(homeName)
		if err != nil {
			return nil, fmt.Errorf("row home %q: %w", homeName, err)
		}

		awayScoreStr := strings.TrimSpace(record[r.scoreCols[0]])
		homeScoreStr := strings.TrimSpace(record[r.scoreCols[1]])

		awayScore, err := safeAtoi(awayScoreStr)
		if err != nil {
			return nil, fmt.Errorf("row away score %q: %w", awayScoreStr, err)
		}
		homeScore, err := safeAtoi(homeScoreStr)
		if err != nil {
			return nil, fmt.Errorf("row home score %q: %w", homeScoreStr, err)
		}

		// Parse start time if available, else default to 7pm ET
		commenceTime := gameDate.Time.Add(19 * time.Hour)
		if etStr := r.idx.str(record, "Start Time (ET)"); etStr != "" {
			if parsed, pErr := parseETTime(gameDate.Time, etStr); pErr == nil {
				commenceTime = parsed
			}
		}

		row := &AsPlayedRow{
			GameDate:     gameDate,
			CommenceTime: commenceTime,
			HomeTeam:     home,
			AwayTeam:     away,
			HomeScore:    homeScore,
			AwayScore:    awayScore,
			Status:       status,
		}

		if goalieCol, ok := r.idx["Home Goalie"]; ok && goalieCol < len(record) {
			row.HomeGoalie = strings.TrimSpace(record[goalieCol])
		}
		if goalieCol, ok := r.idx["Visitor Goalie"]; ok && goalieCol < len(record) {
			row.AwayGoalie = strings.TrimSpace(record[goalieCol])
		}

		if r.hasOdds {
			row.HasOdds = true

			homeML, err := r.idx.intVal(record, "Home ML")
			if err != nil {
				return nil, fmt.Errorf("row Home ML: %w", err)
			}
			awayML, err := r.idx.intVal(record, "Away ML")
			if err != nil {
				return nil, fmt.Errorf("row Away ML: %w", err)
			}
			row.HomeMoneyLine = homeML
			row.AwayMoneyLine = awayML

			spreadF := r.idx.float(record, "Home PL Spread")
			if spreadF != nil {
				row.HomeSpread = *spreadF
			}

			plAway, err := r.idx.intVal(record, "PL Away")
			if err != nil {
				return nil, fmt.Errorf("row PL Away: %w", err)
			}
			plHome, err := r.idx.intVal(record, "PL Home")
			if err != nil {
				return nil, fmt.Errorf("row PL Home: %w", err)
			}
			row.AwaySpreadLine = plAway
			row.HomeSpreadLine = plHome

			ouF := r.idx.float(record, "O/U")
			if ouF != nil {
				row.OverUnder = *ouF
			}
			overLine, err := r.idx.intVal(record, "Over")
			if err != nil {
				return nil, fmt.Errorf("row Over: %w", err)
			}
			underLine, err := r.idx.intVal(record, "Under")
			if err != nil {
				return nil, fmt.Errorf("row Under: %w", err)
			}
			row.OverLine = overLine
			row.UnderLine = underLine
		}

		return row, nil
	}
}

// HomeImpliedProbability returns the implied probability for the home moneyline.
func (r AsPlayedRow) HomeImpliedProbability() float64 {
	return AmericanToImpliedProbability(r.HomeMoneyLine)
}

// AwayImpliedProbability returns the implied probability for the away moneyline.
func (r AsPlayedRow) AwayImpliedProbability() float64 {
	return AmericanToImpliedProbability(r.AwayMoneyLine)
}

func parseISODate(s string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("parse ISO date %q: %w", s, err)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func parseETTime(date time.Time, etStr string) (time.Time, error) {
	// Parse "5:00 PM" format
	t, err := time.Parse("3:04 PM", strings.TrimSpace(etStr))
	if err != nil {
		return time.Time{}, err
	}
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback: ET is UTC-5
		return time.Date(date.Year(), date.Month(), date.Day(),
			t.Hour(), t.Minute(), 0, 0, time.FixedZone("ET", -5*3600)), nil
	}
	return time.Date(date.Year(), date.Month(), date.Day(),
		t.Hour(), t.Minute(), 0, 0, et), nil
}

func safeAtoi(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	v, err := parseInt(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func parseInt(s string) (int, error) {
	// Handle potential float format
	if strings.Contains(s, ".") {
		f, err := parseFloatToInt(s)
		return f, err
	}
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func parseFloatToInt(s string) (int, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return int(f), err
}
