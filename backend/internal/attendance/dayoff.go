package attendance

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Weekly off days and published holidays (lifecycle audit QA-12). New
// attendance cannot be recorded on them; attendance already recorded stays
// as it is, can be read and can still be corrected.

// ErrDayOff matches every *DayOffError.
var ErrDayOff = errors.New("no attendance on a weekly off day or holiday")

// DayOffError says why a date takes no attendance.
type DayOffError struct{ Day DayStatus }

func (e *DayOffError) Error() string        { return ErrDayOff.Error() }
func (e *DayOffError) Is(target error) bool { return target == ErrDayOff }

// DayStatus describes one date. Weekday is 0 (Sunday) … 6 (Saturday).
type DayStatus struct {
	Date          string `json:"date"`
	Open          bool   `json:"open"`
	Reason        string `json:"reason,omitempty"` // "weekly_off" or "holiday"
	Weekday       int    `json:"weekday"`
	HolidayEN     string `json:"holiday_title_en,omitempty"`
	HolidayBN     string `json:"holiday_title_bn,omitempty"`
	WeeklyOffDays []int  `json:"weekly_off_days"`
}

// dayStatus reads whether date takes attendance. A published holiday is
// reported before a weekly off day falling on the same date.
func dayStatus(db *gorm.DB, date string) (DayStatus, error) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return DayStatus{}, ErrInvalid
	}
	out := DayStatus{Date: date, Open: true, Weekday: int(d.Weekday()), WeeklyOffDays: []int{}}
	if err := db.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY day_of_week").Scan(&out.WeeklyOffDays).Error; err != nil {
		return DayStatus{}, err
	}
	var holiday struct{ TitleEN, TitleBN string }
	if err := db.Raw(`SELECT title_en, title_bn FROM school_events
 WHERE kind = 'holiday' AND status = 'published' AND ?::date BETWEEN starts_on AND ends_on
 ORDER BY starts_on, created_at LIMIT 1`, date).Scan(&holiday).Error; err != nil {
		return DayStatus{}, err
	}
	switch {
	case holiday.TitleEN != "":
		out.Open, out.Reason, out.HolidayEN, out.HolidayBN = false, "holiday", holiday.TitleEN, holiday.TitleBN
	default:
		for _, w := range out.WeeklyOffDays {
			if w == out.Weekday {
				out.Open, out.Reason = false, "weekly_off"
			}
		}
	}
	return out, nil
}

// requireOpenDay refuses new attendance on a closed date.
func requireOpenDay(tx *gorm.DB, date string) error {
	day, err := dayStatus(tx, date)
	if err != nil {
		return err
	}
	if !day.Open {
		return &DayOffError{Day: day}
	}
	return nil
}

// Day reports whether a date takes attendance, for the attendance screens.
func (r *GormRepository) Day(ctx context.Context, date string) (DayStatus, error) {
	return dayStatus(r.db.WithContext(ctx), date)
}

// Day reports whether a date takes attendance.
func (s *Service) Day(ctx context.Context, date string) (DayStatus, error) {
	if !validDate(date) {
		return DayStatus{}, ErrInvalid
	}
	return s.repo.Day(ctx, date)
}
