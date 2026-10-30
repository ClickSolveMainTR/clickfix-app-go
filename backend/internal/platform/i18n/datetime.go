package i18n

import (
	"fmt"
	"time"
)

var trMonths = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
var trDays = [...]string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"}

// FormatDateTime is the long date and time used in e-mails and notifications,
// in loc (the organization's time zone):
// tr "2 Kasım 2026 Pazartesi, 10:00", en "Monday, November 2, 2026, 10:00 AM".
// The caller adds the time zone name next to it.
func FormatDateTime(t time.Time, loc *time.Location, locale Locale) string {
	if loc == nil {
		loc = time.UTC
	}
	l := t.In(loc)
	if locale == LocaleTR {
		return fmt.Sprintf("%d %s %d %s, %s", l.Day(), trMonths[l.Month()-1], l.Year(), trDays[l.Weekday()], l.Format("15:04"))
	}
	return l.Format("Monday, January 2, 2006, 3:04 PM")
}

// FormatClock is the time of day: tr "14:30", en "2:30 PM".
func FormatClock(t time.Time, loc *time.Location, locale Locale) string {
	if loc == nil {
		loc = time.UTC
	}
	if locale == LocaleTR {
		return t.In(loc).Format("15:04")
	}
	return t.In(loc).Format("3:04 PM")
}
