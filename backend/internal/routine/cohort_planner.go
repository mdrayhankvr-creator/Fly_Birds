package routine

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type cohortRow struct {
	ID          string
	SessionID   string
	ClassID     string
	SubjectID   string
	TeacherID   string
	DayOfWeek   int
	StartMinute int
	EndMinute   int
	Room        string
	StudentIDs  string
}

func (s *ClassPlannerService) currentCombined(tx *gorm.DB, session string) ([]CombinedInput, error) {
	rows := []cohortRow{}
	err := tx.Raw(`SELECT l.id,l.session_id,l.class_id,l.subject_id,l.teacher_id,l.day_of_week,l.start_minute,l.end_minute,l.room,
coalesce(string_agg(m.student_id::text,',' ORDER BY m.student_id::text),'') AS student_ids
FROM class_cohort_lessons l LEFT JOIN class_cohort_members m ON m.lesson_id=l.id
WHERE l.session_id=? AND l.is_current GROUP BY l.id ORDER BY l.day_of_week,l.start_minute,l.id`, session).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]CombinedInput, 0, len(rows))
	for _, row := range rows {
		students := []string{}
		if row.StudentIDs != "" {
			students = strings.Split(row.StudentIDs, ",")
		}
		out = append(out, CombinedInput{ClassID: row.ClassID, SubjectID: row.SubjectID, TeacherID: row.TeacherID,
			DayOfWeek: row.DayOfWeek, StartTime: clock(row.StartMinute), EndTime: clock(row.EndMinute), Room: row.Room, StudentIDs: students})
	}
	return out, nil
}

func sameTime(dayA, startA, endA, dayB, startB, endB int) bool {
	return dayA == dayB && startA < endB && endA > startB
}

func (s *ClassPlannerService) validateCombined(tx *gorm.DB, p *ClassPlanPreview, in ClassPlan, c PlanCatalogue,
	sections map[string]ClassPlanSection, subjects, assignments, rooms map[string]bool,
	unavailable map[string][][2]int, allowed map[string]bool, counts map[string]int, used map[string]bool, other []ClassInput) error {
	capacityRows := []struct {
		Name     string
		Capacity *int
	}{}
	if err := tx.Raw("SELECT name,capacity FROM exam_rooms WHERE is_active").Scan(&capacityRows).Error; err != nil {
		return err
	}
	capacities := map[string]*int{}
	for _, row := range capacityRows {
		capacities[normalizedRoom(row.Name)] = row.Capacity
	}
	otherRows := []cohortRow{}
	if err := tx.Raw(`SELECT l.id,l.session_id,l.class_id,l.subject_id,l.teacher_id,l.day_of_week,l.start_minute,l.end_minute,l.room,
coalesce(string_agg(m.student_id::text,',' ORDER BY m.student_id::text),'') AS student_ids
FROM class_cohort_lessons l LEFT JOIN class_cohort_members m ON m.lesson_id=l.id
JOIN academic_sessions a ON a.id=l.session_id JOIN academic_sessions selected ON selected.id=?
WHERE l.is_current AND l.session_id<>? AND a.start_date<=selected.end_date AND a.end_date>=selected.start_date
GROUP BY l.id`, in.SessionID, in.SessionID).Scan(&otherRows).Error; err != nil {
		return err
	}
	totalMembers := 0
	for i, v := range in.CombinedSlots {
		slot := len(in.Slots) + i + 1
		a, okA := minute(v.StartTime)
		b, okB := minute(v.EndTime)
		if !uuid.MatchString(v.ClassID) || !uuid.MatchString(v.SubjectID) || !uuid.MatchString(v.TeacherID) || !okA || !okB || a >= b || v.DayOfWeek < 0 || v.DayOfWeek > 6 || len(v.StudentIDs) == 0 || len(v.StudentIDs) > 500 {
			p.Issues = append(p.Issues, PlanIssue{Code: "invalid_cohort", Message: "Choose a class, subject, teacher, period and explicit roster", Slot: slot})
			continue
		}
		totalMembers += len(v.StudentIDs)
		if totalMembers > 10000 {
			return ErrInvalid
		}
		if !allowed[fmt.Sprintf("%d|%s|%s", v.DayOfWeek, v.StartTime, v.EndTime)] {
			p.Issues = append(p.Issues, PlanIssue{Code: "outside_period", Message: "Cohort lesson must match a teaching period outside lunch and holidays", Slot: slot})
		}
		if !subjects[v.ClassID+"|"+v.SubjectID] || !assignments[v.ClassID+"|"+v.SubjectID+"|"+v.TeacherID] {
			p.Issues = append(p.Issues, PlanIssue{Code: "assignment_missing", Message: "Cohort subject and teacher assignment must match the class", Slot: slot})
		}
		room := normalizedRoom(v.Room)
		if !rooms[room] {
			p.Issues = append(p.Issues, PlanIssue{Code: "room_missing", Message: "Choose an active registered room", Slot: slot})
		}
		if rooms[room] && capacities[room] == nil {
			p.Issues = append(p.Issues, PlanIssue{Code: "room_capacity_missing", Message: "Set the room capacity before a cohort lesson", Slot: slot})
		}
		if cap := capacities[room]; cap != nil && len(v.StudentIDs) > *cap {
			p.Issues = append(p.Issues, PlanIssue{Code: "room_capacity", Message: "Cohort roster exceeds room capacity", Slot: slot})
		}
		for _, block := range unavailable[fmt.Sprintf("%s|%d", v.TeacherID, v.DayOfWeek)] {
			if a < block[1] && b > block[0] {
				p.Issues = append(p.Issues, PlanIssue{Code: "teacher_unavailable", Message: "Teacher is unavailable during this cohort lesson", Slot: slot})
			}
		}
		seen := map[string]bool{}
		for _, id := range v.StudentIDs {
			if !uuid.MatchString(id) || seen[id] {
				p.Issues = append(p.Issues, PlanIssue{Code: "duplicate_or_invalid_student", Message: "Cohort roster needs distinct valid student IDs", Slot: slot})
				break
			}
			seen[id] = true
		}
		memberRows := []struct {
			StudentID, SectionID, ClassID string
			Eligible                      bool
		}{}
		if err := tx.Raw(`SELECT en.student_id,en.section_id,en.class_id,student_takes_subject(en.student_id,en.session_id,en.class_id,?::uuid) AS eligible
FROM student_enrolments en WHERE en.session_id=? AND en.class_id=? AND en.state='current' AND en.student_status='active' AND en.student_id IN ?`, v.SubjectID, in.SessionID, v.ClassID, v.StudentIDs).Scan(&memberRows).Error; err != nil {
			return err
		}
		sectionIDs := map[string]bool{}
		if len(memberRows) != len(seen) {
			p.Issues = append(p.Issues, PlanIssue{Code: "roster_not_current", Message: "Every cohort member needs a current enrolment in this class", Slot: slot})
		}
		for _, member := range memberRows {
			if !member.Eligible {
				p.Issues = append(p.Issues, PlanIssue{Code: "subject_choice_missing", Message: "A cohort member has not chosen or is not eligible for this subject", Slot: slot})
			}
			if sec, ok := sections[member.SectionID]; !ok || sec.ClassID != v.ClassID {
				p.Issues = append(p.Issues, PlanIssue{Code: "section_inactive", Message: "Cohort member must belong to an active section of this class", Slot: slot})
			}
			sectionIDs[member.SectionID] = true
		}
		for _, ordinary := range in.Slots {
			x, xok := minute(ordinary.StartTime)
			y, yok := minute(ordinary.EndTime)
			if xok && yok && sameTime(v.DayOfWeek, a, b, ordinary.DayOfWeek, x, y) {
				if v.TeacherID == ordinary.TeacherID || room == normalizedRoom(ordinary.Room) || sectionIDs[ordinary.SectionID] {
					p.Issues = append(p.Issues, PlanIssue{Code: "overlap_section_lesson", Message: "Cohort overlaps a section lesson, teacher or room", Slot: slot})
				}
			}
		}
		for _, ordinary := range other {
			x, xok := minute(ordinary.StartTime)
			y, yok := minute(ordinary.EndTime)
			if xok && yok && sameTime(v.DayOfWeek, a, b, ordinary.DayOfWeek, x, y) && (v.TeacherID == ordinary.TeacherID || room == normalizedRoom(ordinary.Room)) {
				p.Issues = append(p.Issues, PlanIssue{Code: "overlap_existing_session", Message: "Teacher or room overlaps another academic session", Slot: slot})
			}
		}
		for j := 0; j < i; j++ {
			prior := in.CombinedSlots[j]
			x, xok := minute(prior.StartTime)
			y, yok := minute(prior.EndTime)
			if !xok || !yok || !sameTime(v.DayOfWeek, a, b, prior.DayOfWeek, x, y) {
				continue
			}
			conflict := v.TeacherID == prior.TeacherID || room == normalizedRoom(prior.Room)
			for _, id := range prior.StudentIDs {
				if seen[id] {
					conflict = true
					break
				}
			}
			if conflict {
				p.Issues = append(p.Issues, PlanIssue{Code: "overlap_cohort", Message: "Parallel cohorts need disjoint students and different teachers and rooms", Slot: slot})
			}
		}
		for _, prior := range otherRows {
			if sameTime(v.DayOfWeek, a, b, prior.DayOfWeek, prior.StartMinute, prior.EndMinute) &&
				(v.TeacherID == prior.TeacherID || room == normalizedRoom(prior.Room) || cohortMemberOverlap(seen, prior.StudentIDs)) {
				p.Issues = append(p.Issues, PlanIssue{Code: "overlap_existing_cohort", Message: "Cohort overlaps an active lesson in another academic session", Slot: slot})
			}
		}
		for sectionID := range sectionIDs {
			counts[sectionID+"|"+v.SubjectID]++
			used[fmt.Sprintf("%s|%d|%s|%s", sectionID, v.DayOfWeek, v.StartTime, v.EndTime)] = true
		}
	}
	return nil
}

func cohortMemberOverlap(ids map[string]bool, csv string) bool {
	for _, id := range strings.Split(csv, ",") {
		if ids[id] {
			return true
		}
	}
	return false
}
