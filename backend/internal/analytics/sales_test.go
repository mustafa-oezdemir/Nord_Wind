package analytics

import (
	"testing"
	"time"
)

func TestReportRangeUsesBerlinCalendar(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 31, 15, 0, 0, 0, location)
	for _, test := range []struct {
		period Period
		first  string
		last   string
		count  int
		unit   string
	}{
		{PeriodWeek, "2026-10-25", "2026-10-31", 7, "day"},
		{PeriodMonth, "2026-10-02", "2026-10-31", 30, "day"},
		{PeriodQuarter, "2026-08-01", "2026-10-01", 3, "month"},
	} {
		start, end, buckets, unit := reportRange(now, test.period)
		if len(buckets) != test.count || buckets[0].Key != test.first ||
			buckets[len(buckets)-1].Key != test.last || unit != test.unit ||
			!start.Before(end) {
			t.Errorf("%s: Zeitraum %s bis %s, %d Buckets, %s", test.period,
				buckets[0].Key, buckets[len(buckets)-1].Key, len(buckets), unit)
		}
	}
}
