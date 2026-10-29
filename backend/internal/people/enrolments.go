package people

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ErrHistory: the change would rewrite a year that already has records
// (for example moving a student back into a session they already have, or
// changing class while marks exist in the old class). Use promotion or a
// section transfer instead.
var ErrHistory = errors.New("the change would rewrite enrolment history")

// Enrolment is one academic year of a student: where they were and how the
// year ended. Events list what changed during it, oldest first.
type Enrolment struct {
	ID            string           `json:"id"`
	SessionID     string           `json:"session_id"`
	SessionName   string           `json:"session_name"`
	ClassID       string           `json:"class_id"`
	ClassName     string           `json:"class_name"`
	SectionID     string           `json:"section_id"`
	SectionName   string           `json:"section_name"`
	RollNo        *int             `json:"roll_no"`
	State         string           `json:"state"`
	StudentStatus string           `json:"student_status"`
	Outcome       *string          `json:"outcome"`
	Source        string           `json:"source"`
	StartedOn     *string          `json:"started_on"`
	EndedOn       *string          `json:"ended_on"`
	Events        []EnrolmentEvent `json:"events" gorm:"-"`
}

type EnrolmentEvent struct {
	EnrolmentID     string  `json:"-"`
	Kind            string  `json:"kind"`
	FromClassName   *string `json:"from_class_name"`
	ToClassName     *string `json:"to_class_name"`
	FromSectionName *string `json:"from_section_name"`
	ToSectionName   *string `json:"to_section_name"`
	FromValue       string  `json:"from_value"`
	ToValue         string  `json:"to_value"`
	// Who made the change; only shown to the administrator and head teacher.
	ActorName  *string `json:"actor_name,omitempty"`
	OccurredAt string  `json:"occurred_at"`
}

// Enrolments returns a student's enrolment history, oldest year first, to
// anyone who may read the student (the same scoping as GetStudent).
func (r *GormRepository) Enrolments(ctx context.Context, a Actor, id string) ([]Enrolment, error) {
	if _, err := r.GetStudent(ctx, a, id); err != nil {
		return nil, err
	}
	db := r.db.WithContext(ctx)
	out := []Enrolment{}
	if err := db.Raw(`SELECT e.id, e.session_id, se.name AS session_name, e.class_id, c.name AS class_name, e.section_id, sec.name AS section_name,
       e.roll_no, e.state, e.student_status, e.outcome, e.source,
       to_char(e.started_on, 'YYYY-MM-DD') AS started_on, to_char(e.ended_on, 'YYYY-MM-DD') AS ended_on
  FROM student_enrolments e JOIN academic_sessions se ON se.id = e.session_id
  JOIN academic_classes c ON c.id = e.class_id JOIN academic_sections sec ON sec.id = e.section_id
 WHERE e.student_id = ? ORDER BY se.start_date, se.name, e.id`, id).Scan(&out).Error; err != nil {
		return nil, err
	}
	events := []EnrolmentEvent{}
	if err := db.Raw(`SELECT v.enrolment_id, v.kind, fc.name AS from_class_name, tc.name AS to_class_name,
       fs.name AS from_section_name, ts.name AS to_section_name, v.from_value, v.to_value,
       CASE WHEN ? THEN coalesce(nullif(u.display_name, ''), u.email) END AS actor_name,
       to_char(v.occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS occurred_at
  FROM student_enrolment_events v
  LEFT JOIN academic_classes fc ON fc.id = v.from_class_id LEFT JOIN academic_classes tc ON tc.id = v.to_class_id
  LEFT JOIN academic_sections fs ON fs.id = v.from_section_id LEFT JOIN academic_sections ts ON ts.id = v.to_section_id
  LEFT JOIN users u ON u.id = v.actor_id
 WHERE v.student_id = ? ORDER BY v.occurred_at, v.id`, a.Role == "admin" || a.Role == "head_teacher", id).Scan(&events).Error; err != nil {
		return nil, err
	}
	at := map[string]int{}
	for i := range out {
		out[i].Events = []EnrolmentEvent{}
		at[out[i].ID] = i
	}
	for _, v := range events {
		if i, ok := at[v.EnrolmentID]; ok {
			out[i].Events = append(out[i].Events, v)
		}
	}
	return out, nil
}

// setActor records the acting user for the enrolment triggers of this
// transaction (student_enrolment_events.actor_id).
func setActor(tx *gorm.DB, a Actor) error {
	return tx.Exec("SELECT set_config('flybirds.actor_id', ?, true)", a.ID).Error
}

// historyError maps the enrolment triggers' refusal (SQLSTATE FB001).
func historyError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "FB001" {
		return ErrHistory
	}
	return err
}
