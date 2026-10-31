package analytics

import (
	"errors"
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
		query       Query
		first, last string
		count       int
		unit        string
		lastLabel   string
	}{
		{Query{Period: PeriodWeek}, "2026-10-25", "2026-10-31", 7, "day", "31.10."},
		{Query{Period: PeriodMonth}, "2026-10-02", "2026-10-31", 30, "day", "31.10."},
		{Query{Period: PeriodQuarter}, "2026-08-01", "2026-10-01", 3, "month", "Okt 2026"},
		// Eigener Zeitraum über die Zeitumstellung am 25. Oktober: ein Bucket je Kalendertag.
		{Query{Period: PeriodCustom, From: "2026-10-20", To: "2026-10-26"}, "2026-10-20", "2026-10-26", 7, "day", "26.10."},
		{Query{Period: PeriodCustom, From: "2026-10-31", To: "2026-10-31"}, "2026-10-31", "2026-10-31", 1, "day", "31.10."},
		// Über den Jahreswechsel erhalten Tage die Jahreszahl.
		{Query{Period: PeriodCustom, From: "2025-12-30", To: "2026-01-02"}, "2025-12-30", "2026-01-02", 4, "day", "02.01.26"},
		// Ab 63 Tagen werden Monate gezeigt; der erste Monat beginnt mit seinem Ersten.
		{Query{Period: PeriodCustom, From: "2026-05-15", To: "2026-10-31"}, "2026-05-01", "2026-10-01", 6, "month", "Okt 2026"},
	} {
		start, end, buckets, unit, err := reportRange(now, test.query)
		if err != nil {
			t.Fatalf("%+v: %v", test.query, err)
		}
		if len(buckets) != test.count || buckets[0].Key != test.first || buckets[len(buckets)-1].Key != test.last ||
			unit != test.unit || !start.Before(end) || buckets[len(buckets)-1].Label != test.lastLabel {
			t.Errorf("%+v: %s bis %s, %d Buckets, %s, %q", test.query, buckets[0].Key, buckets[len(buckets)-1].Key,
				len(buckets), unit, buckets[len(buckets)-1].Label)
		}
	}
	start, _, _, _, _ := reportRange(now, Query{Period: PeriodCustom, From: "2026-05-15", To: "2026-10-31"})
	if start.Format("2006-01-02") != "2026-05-15" {
		t.Errorf("Monatsansicht muss am gewählten Tag beginnen: %s", start)
	}
	for _, invalid := range []Query{
		{Period: PeriodCustom, From: "2026-10-31", To: "2026-10-30"},
		{Period: PeriodCustom, From: "2026-10-01", To: "2026-11-01"},
		{Period: PeriodCustom, From: "2024-10-30", To: "2026-10-31"},
		{Period: PeriodCustom, From: "31.10.2026", To: "2026-10-31"},
		{Period: PeriodCustom},
		{Period: PeriodWeek, From: "2026-10-01"},
		{Period: "year"},
	} {
		if _, _, _, _, err := reportRange(now, invalid); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v akzeptiert", invalid)
		}
	}
}

func TestDescendantsIncludeEveryLevel(t *testing.T) {
	categories := []CatalogCategory{
		{ID: "kueche"}, {ID: "pfannen", ParentID: "kueche"}, {ID: "guss", ParentID: "pfannen"}, {ID: "garten"},
	}
	scope := descendants(categories, "kueche")
	if !scope["kueche"] || !scope["pfannen"] || !scope["guss"] || scope["garten"] {
		t.Fatalf("Unterkategorien: %v", scope)
	}
	if leaf := descendants(categories, "guss"); len(leaf) != 1 || !leaf["guss"] {
		t.Fatalf("Unterkategorie allein: %v", leaf)
	}
}
