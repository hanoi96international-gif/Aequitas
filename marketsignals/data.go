package marketsignals

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadCSV reads bars from a CSV file. The expected columns are:
//
//	time,open,high,low,close,volume[,buy_volume,sell_volume][,funding]
//
// time is either an RFC3339 timestamp or a Unix epoch in seconds or
// milliseconds. A header row is detected and skipped. The taker-split and
// funding columns are optional; when they are absent the flow and positioning
// agents stand down rather than inventing the missing input.
func LoadCSV(path, symbol string, interval time.Duration) (*Series, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // rows may carry the optional trailing columns or not
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: empty file", path)
	}

	// Detect a header by trying to parse the first cell as a timestamp.
	start := 0
	if _, err := parseTime(rows[0][0]); err != nil {
		start = 1
	}

	s := &Series{Symbol: symbol, Interval: interval}
	for i := start; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 6 {
			return nil, fmt.Errorf("%s line %d: got %d columns, need at least 6", path, i+1, len(row))
		}
		ts, err := parseTime(row[0])
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		nums := make([]float64, 0, len(row)-1)
		for j := 1; j < len(row); j++ {
			cell := strings.TrimSpace(row[j])
			if cell == "" {
				nums = append(nums, 0)
				continue
			}
			v, err := strconv.ParseFloat(cell, 64)
			if err != nil {
				return nil, fmt.Errorf("%s line %d column %d: %w", path, i+1, j+1, err)
			}
			nums = append(nums, v)
		}

		c := Candle{Time: ts, Open: nums[0], High: nums[1], Low: nums[2], Close: nums[3], Volume: nums[4]}
		if len(nums) >= 7 {
			c.BuyVolume, c.SellVolume = nums[5], nums[6]
		}
		s.Candles = append(s.Candles, c)
		if len(nums) >= 8 {
			s.Funding = append(s.Funding, nums[7])
		}
	}

	// A partially populated funding column is worse than none: it would let
	// the positioning agent read zeros as "funding is neutral" when the truth
	// is that nobody recorded it.
	if len(s.Funding) > 0 && len(s.Funding) != len(s.Candles) {
		return nil, fmt.Errorf("%s: funding present on %d of %d rows — fill the column or drop it",
			path, len(s.Funding), len(s.Candles))
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func parseTime(cell string) (time.Time, error) {
	cell = strings.TrimSpace(cell)
	if t, err := time.Parse(time.RFC3339, cell); err == nil {
		return t.UTC(), nil
	}
	n, err := strconv.ParseInt(cell, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot read %q as RFC3339 or a Unix epoch", cell)
	}
	// Anything past ~2001 in milliseconds is beyond year 33658 in seconds, so
	// the magnitude disambiguates the two conventions unambiguously here.
	if n > 1e12 {
		return time.UnixMilli(n).UTC(), nil
	}
	return time.Unix(n, 0).UTC(), nil
}

// LoadLaunches reads a JSON array of Launch objects.
func LoadLaunches(path string) ([]Launch, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Launch
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
