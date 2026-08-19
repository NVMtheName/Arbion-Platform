package strategy

import (
	"time"
	_ "time/tzdata"
)

const (
	firstPublishedCalendarYear = 2026
	lastPublishedCalendarYear  = 2028
)

var easternLocation = func() *time.Location {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("embedded America/New_York timezone unavailable")
	}
	return location
}()

// These dates are the closures and early closes published by NYSE for
// 2026-2028. The scheduler deliberately fails closed outside this reviewed
// horizon so a stale calendar cannot silently become trading authority.
// Source: https://www.nyse.com/trade/hours-calendars
var nyseClosedDates = map[string]struct{}{
	"2026-01-01": {},
	"2026-01-19": {},
	"2026-02-16": {},
	"2026-04-03": {},
	"2026-05-25": {},
	"2026-06-19": {},
	"2026-07-03": {},
	"2026-09-07": {},
	"2026-11-26": {},
	"2026-12-25": {},
	"2027-01-01": {},
	"2027-01-18": {},
	"2027-02-15": {},
	"2027-03-26": {},
	"2027-05-31": {},
	"2027-06-18": {},
	"2027-07-05": {},
	"2027-09-06": {},
	"2027-11-25": {},
	"2027-12-24": {},
	"2028-01-17": {},
	"2028-02-21": {},
	"2028-04-14": {},
	"2028-05-29": {},
	"2028-06-19": {},
	"2028-07-04": {},
	"2028-09-04": {},
	"2028-11-23": {},
	"2028-12-25": {},
}

var nyseEarlyCloseDates = map[string]struct{}{
	"2026-11-27": {},
	"2026-12-24": {},
	"2027-11-26": {},
	"2028-07-03": {},
	"2028-11-24": {},
}

func regularSessionWindow(at time.Time) (time.Time, time.Time, bool) {
	local := at.In(easternLocation)
	if local.Year() < firstPublishedCalendarYear || local.Year() > lastPublishedCalendarYear {
		return time.Time{}, time.Time{}, false
	}
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return time.Time{}, time.Time{}, false
	}
	dateKey := local.Format("2006-01-02")
	if _, closed := nyseClosedDates[dateKey]; closed {
		return time.Time{}, time.Time{}, false
	}

	closeHour, closeMinute := 15, 55
	if _, earlyClose := nyseEarlyCloseDates[dateKey]; earlyClose {
		closeHour, closeMinute = 12, 55
	}
	open := time.Date(local.Year(), local.Month(), local.Day(), 9, 35, 0, 0, easternLocation)
	close := time.Date(local.Year(), local.Month(), local.Day(), closeHour, closeMinute, 0, 0, easternLocation)
	return open, close, true
}

func inRegularSession(at time.Time) bool {
	open, close, openDay := regularSessionWindow(at)
	if !openDay {
		return false
	}
	local := at.In(easternLocation)
	return !local.Before(open) && local.Before(close)
}

func nextRegularSession(after time.Time) (time.Time, bool) {
	local := after.In(easternLocation)
	if local.Year() < firstPublishedCalendarYear || local.Year() > lastPublishedCalendarYear {
		return time.Time{}, false
	}
	date := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, easternLocation)
	for !date.After(time.Date(lastPublishedCalendarYear, time.December, 31, 23, 59, 59, 0, easternLocation)) {
		open, _, openDay := regularSessionWindow(date)
		if openDay && open.After(local) {
			return open.UTC(), true
		}
		date = date.AddDate(0, 0, 1)
	}
	return time.Time{}, false
}
