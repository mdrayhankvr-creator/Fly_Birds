package exams

import (
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// examRules is how an exam's results are calculated: today's rules for an
// exam made outside a blueprint (and for legacy versions, whose policy is the
// default), otherwise its blueprint version's policy and combined papers.
type examRules struct {
	Policy   ScorePolicy
	Dense    bool              // dense ranking instead of competition
	Combined map[string]string // subject id → combined key
	Choices  bool              // the class uses per-student optional subjects
}

func rulesFor(tx *gorm.DB, e Exam) (examRules, error) {
	r := examRules{Combined: map[string]string{}}
	if e.BlueprintVersionID != nil {
		var v struct{ GPARounding, AbsentPolicy, RankMode, PaperRule string }
		if err := tx.Raw("SELECT gpa_rounding, absent_policy, rank_mode, paper_rule FROM blueprint_versions WHERE id = ?", *e.BlueprintVersionID).Scan(&v).Error; err != nil {
			return r, err
		}
		if v.GPARounding == "truncate" {
			r.Policy.GPA = calc.Truncate
		}
		if v.AbsentPolicy == "missing" {
			r.Policy.Absent = calc.AbsentIsMissing
		}
		r.Policy.EachPaper = v.PaperRule == "each_paper"
		r.Dense = v.RankMode == "dense"
		rows := []struct{ SubjectID, CombinedKey string }{}
		if err := tx.Raw("SELECT subject_id, combined_key FROM blueprint_subjects WHERE version_id = ? AND combined_key IS NOT NULL", *e.BlueprintVersionID).Scan(&rows).Error; err != nil {
			return r, err
		}
		for _, row := range rows {
			r.Combined[row.SubjectID] = row.CombinedKey
		}
	}
	err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM student_subject_choices c JOIN students s ON s.id = c.student_id
 WHERE c.session_id = ? AND s.session_id = ? AND s.class_id = ?)`, e.SessionID, e.SessionID, e.ClassID).Row().Scan(&r.Choices)
	return r, err
}

// lineRow is one stored mark of a student in an exam.
type lineRow struct {
	MarkID    string
	SubjectID string
	Marks     *float64
	Status    string
	FullMarks float64
	PassMarks float64
	IsFourth  bool
}

// studentLines loads a student's marks for the subjects they take, with
// component marks, and how many subjects they take in the exam.
func studentLines(tx *gorm.DB, e Exam, studentID string, rules examRules) ([]lineRow, []MarkLine, int, error) {
	rows := []lineRow{}
	err := tx.Table("exam_marks m").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").
		Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").
		Select("m.id AS mark_id,es.subject_id,m.marks,m.status,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth").
		Where("m.exam_id=? AND m.student_id=? AND student_takes_subject(m.student_id, ?, ?, es.subject_id)", e.ID, studentID, e.SessionID, e.ClassID).
		Order("es.subject_id").Find(&rows).Error
	if err != nil {
		return nil, nil, 0, err
	}
	var expected int
	if err := tx.Raw("SELECT count(*) FROM exam_subjects es WHERE es.exam_id = ? AND student_takes_subject(?, ?, ?, es.subject_id)", e.ID, studentID, e.SessionID, e.ClassID).Row().Scan(&expected); err != nil {
		return nil, nil, 0, err
	}
	components, err := componentLines(tx, rows)
	if err != nil {
		return nil, nil, 0, err
	}
	lines := make([]MarkLine, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, MarkLine{Marks: row.Marks, FullMarks: row.FullMarks, PassMarks: row.PassMarks, Fourth: row.IsFourth, Status: row.Status,
			Combined: rules.Combined[row.SubjectID], Components: components[row.MarkID]})
	}
	return rows, lines, expected, nil
}

// componentLines loads the component marks (with pass marks) of marks.
func componentLines(tx *gorm.DB, rows []lineRow) (map[string][]ComponentLine, error) {
	out := map[string][]ComponentLine{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.MarkID)
	}
	comps := []struct {
		MarkID string
		Key    string
		Marks  float64
		Pass   *float64
	}{}
	if err := tx.Raw(`SELECT cm.exam_mark_id AS mark_id, cm.component_key AS key, cm.marks::float8 AS marks, c.pass_marks::float8 AS pass
  FROM exam_component_marks cm JOIN exam_subject_components c ON c.exam_subject_id = cm.exam_subject_id AND c.component_key = cm.component_key
 WHERE cm.exam_mark_id IN ? ORDER BY c.display_order`, ids).Scan(&comps).Error; err != nil {
		return nil, err
	}
	for _, c := range comps {
		out[c.MarkID] = append(out[c.MarkID], ComponentLine{Key: c.Key, Marks: c.Marks, Pass: c.Pass})
	}
	return out, nil
}

// classRanks positions the published results of an exam's class: within
// each section and across the class. Exams of one term and assessment are
// ranked together (E1-05); exams made outside a term, by name as before.
func classRanks(tx *gorm.DB, e Exam, dense bool) error {
	fn := "rank()"
	if dense {
		fn = "dense_rank()"
	}
	group, args := "lower(x.name)=lower(?) AND x.exam_type=? AND x.term_id IS NULL", []any{e.SessionID, e.ClassID, e.Name, e.ExamType}
	if e.TermID != nil && e.AssessmentKey != nil {
		group, args = "x.term_id=? AND x.assessment_key=?", []any{e.SessionID, e.ClassID, *e.TermID, *e.AssessmentKey}
	}
	return tx.Exec(`WITH ranked AS (
 SELECT r.exam_id,r.student_id,
 `+fn+` OVER (ORDER BY r.gpa DESC,r.total_marks DESC) AS class_rank,
 `+fn+` OVER (PARTITION BY r.section_id ORDER BY r.gpa DESC,r.total_marks DESC) AS section_rank
 FROM exam_results r JOIN exams x ON x.id=r.exam_id
 WHERE x.session_id=? AND x.class_id=? AND `+group+` AND x.status IN ('published','locked')
 ) UPDATE exam_results r SET class_position=ranked.class_rank,section_position=ranked.section_rank
 FROM ranked WHERE r.exam_id=ranked.exam_id AND r.student_id=ranked.student_id`, args...).Error
}

// linkLegacyTerm keeps an exam made outside the setup wizard with the exams
// the upgrade linked: one named and typed like an upgraded term joins that
// term (and its class's legacy assessment), so class positions group the
// sections as they did by name (E1-05). Exams the wizard made are untouched.
func linkLegacyTerm(tx *gorm.DB, examID string) error {
	if err := tx.Exec(`UPDATE exams SET term_id = NULL, assessment_key = NULL, blueprint_version_id = NULL
 WHERE id = ? AND term_id IN (SELECT id FROM exam_terms WHERE legacy_key IS NOT NULL)`, examID).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE exams e SET term_id = t.id,
       assessment_key = coalesce((SELECT ba.key FROM blueprint_assessments ba JOIN blueprint_versions v ON v.id = ba.version_id
                                    JOIN assessment_blueprints b ON b.id = v.blueprint_id
                                   WHERE ba.term_id = t.id AND v.legacy AND v.status = 'active' AND b.session_id = e.session_id AND b.class_id = e.class_id), 'A' || t.sequence),
       blueprint_version_id = (SELECT ba.version_id FROM blueprint_assessments ba JOIN blueprint_versions v ON v.id = ba.version_id
                                 JOIN assessment_blueprints b ON b.id = v.blueprint_id
                                WHERE ba.term_id = t.id AND v.legacy AND v.status = 'active' AND b.session_id = e.session_id AND b.class_id = e.class_id)
  FROM exam_terms t
 WHERE e.id = ? AND e.term_id IS NULL AND t.session_id = e.session_id AND t.legacy_key = lower(e.name) || '|' || e.exam_type`, examID).Error
}
