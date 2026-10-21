package blueprints

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// Term is a term of the blueprint's session, as validation needs it.
type Term struct {
	Name     string
	Sequence int
	Kind     string
}

// ClassSubject is a subject mapped to the blueprint's class.
type ClassSubject struct {
	Name   string
	Fourth bool // the class mapping marks it as a fourth (optional) subject
}

// Context is what a definition is validated against: the session's terms,
// the class's subjects and the school's grading bands.
type Context struct {
	Terms    map[string]Term
	Subjects map[string]ClassSubject
	Bands    []calc.Band
}

// Issue is one validation failure. Code and Params let the interface say it
// in either language; Path points at the field ("subjects.2.terms.0").
type Issue struct {
	Code    string            `json:"code"`
	Path    string            `json:"path"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"message"`
}

var (
	keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,15}$`)
	kinds      = map[string]bool{"class_test": true, "continuous": true, "performance": true, "term_exam": true}
	visibility = map[string]bool{"hidden": true, "after_term_publication": true, "with_assessment_publication": true}
	roles      = map[string]bool{"required": true, "optional": true, "non_graded": true}
	combines   = map[string]bool{"single": true, "sum": true, "average": true, "best": true, "latest": true}
)

// maxSheetFull is the largest full mark a sheet can have (exam_subjects).
const maxSheetFull calc.Marks = 100000

// Validate checks a definition against the rules of the blueprint
// specification (section 6). An empty result means it can be activated.
func Validate(d Definition, c Context) []Issue {
	v := &validator{ctx: c}
	v.assessments(d)
	v.subjects(d)
	v.combined(d)
	v.annual(d)
	v.policy(d)
	v.bands()
	return v.issues
}

type validator struct {
	ctx    Context
	issues []Issue
}

func (v *validator) add(code, path, message string, params ...string) {
	p := map[string]string{}
	for i := 0; i+1 < len(params); i += 2 {
		p[params[i]] = params[i+1]
	}
	if len(p) == 0 {
		p = nil
	}
	v.issues = append(v.issues, Issue{Code: code, Path: path, Params: p, Message: message})
}

func (v *validator) subjectName(id string) string {
	if s, ok := v.ctx.Subjects[id]; ok {
		return s.Name
	}
	return id
}

func (v *validator) termName(id string) string {
	if t, ok := v.ctx.Terms[id]; ok {
		return t.Name
	}
	return id
}

// marks parses a two-decimal value, reporting the field when it cannot.
func (v *validator) marks(value float64, path, label string) (calc.Marks, bool) {
	m, err := calc.MarksOf(value)
	if err != nil {
		v.add("marks_invalid", path, fmt.Sprintf("%s must be a number of marks with at most two decimals", label), "field", label)
		return 0, false
	}
	return m, true
}

func (v *validator) assessments(d Definition) {
	if len(d.Assessments) == 0 {
		v.add("no_assessments", "assessments", "add at least one assessment")
	}
	seen := map[string]bool{}
	for i, a := range d.Assessments {
		path := fmt.Sprintf("assessments.%d", i)
		if !keyPattern.MatchString(a.Key) {
			v.add("assessment_key_invalid", path+".key", "an assessment key is 1–16 capital letters, digits or _ starting with a letter", "key", a.Key)
		} else if seen[a.Key] {
			v.add("assessment_key_duplicate", path+".key", fmt.Sprintf("the key %s is used twice", a.Key), "key", a.Key)
		}
		seen[a.Key] = true
		if name := strings.TrimSpace(a.Name); name == "" || name != a.Name || utf8.RuneCountInString(name) > 80 {
			v.add("assessment_name_invalid", path+".name", "give each assessment a name of up to 80 characters", "key", a.Key)
		}
		if _, ok := v.ctx.Terms[a.TermID]; !ok {
			v.add("assessment_term_unknown", path+".term_id", fmt.Sprintf("%s must belong to a term of this session", a.Key), "key", a.Key)
		}
		if !kinds[a.Kind] {
			v.add("assessment_kind_invalid", path+".kind", "choose class test, continuous, performance or term exam", "key", a.Key)
		}
		if !visibility[a.Visibility] {
			v.add("visibility_invalid", path+".visibility", "choose who may see this assessment", "key", a.Key)
		}
	}
}

func (v *validator) subjects(d Definition) {
	seen := map[string]bool{}
	for i, s := range d.Subjects {
		path := fmt.Sprintf("subjects.%d", i)
		name := v.subjectName(s.SubjectID)
		class, known := v.ctx.Subjects[s.SubjectID]
		switch {
		case !known:
			v.add("subject_unknown", path, "this subject is not taught in the class", "subject", name)
		case seen[s.SubjectID]:
			v.add("subject_duplicate", path, fmt.Sprintf("%s is listed twice", name), "subject", name)
		}
		seen[s.SubjectID] = true
		if !roles[s.Role] {
			v.add("role_invalid", path+".role", fmt.Sprintf("choose a role for %s", name), "subject", name)
			continue
		}
		if known && s.Role == "optional" && !class.Fourth {
			v.add("optional_not_fourth", path+".role", fmt.Sprintf("%s is a required subject of the class; only a fourth subject can be optional", name), "subject", name)
		}
		if known && s.Role == "required" && class.Fourth {
			v.add("fourth_not_optional", path+".role", fmt.Sprintf("%s is the class's fourth subject; make it optional or not graded", name), "subject", name)
		}
		if s.Role == "non_graded" {
			if len(s.Sheets) > 0 || len(s.Terms) > 0 {
				v.add("non_graded_rules", path, fmt.Sprintf("%s is not graded, so it has no sheets or term rules", name), "subject", name)
			}
			continue
		}
		v.sheets(d, s, path, name)
		v.terms(d, s, path, name)
	}
	// Coverage: every class subject is in the blueprint (or explicitly not graded).
	missing := []string{}
	for id, cs := range v.ctx.Subjects {
		if !seen[id] {
			missing = append(missing, cs.Name)
		}
	}
	sort.Strings(missing)
	for _, n := range missing {
		v.add("coverage_missing", "subjects", fmt.Sprintf("%s is taught in the class but not in the blueprint; add it or mark it not graded", n), "subject", n)
	}
}

func (v *validator) sheets(d Definition, s SubjectRule, path, name string) {
	if len(s.Sheets) == 0 {
		v.add("no_sheets", path+".sheets", fmt.Sprintf("%s has no mark sheet in any assessment", name), "subject", name)
	}
	seen := map[string]bool{}
	for j, sh := range s.Sheets {
		sp := fmt.Sprintf("%s.sheets.%d", path, j)
		if _, ok := d.Assessment(sh.Assessment); !ok {
			v.add("sheet_assessment_unknown", sp+".assessment", fmt.Sprintf("%s: %s is not an assessment of the blueprint", name, sh.Assessment), "subject", name, "assessment", sh.Assessment)
		} else if seen[sh.Assessment] {
			v.add("sheet_duplicate", sp+".assessment", fmt.Sprintf("%s has two sheets in %s", name, sh.Assessment), "subject", name, "assessment", sh.Assessment)
		}
		seen[sh.Assessment] = true
		if len(sh.Components) == 0 {
			v.add("no_components", sp+".components", fmt.Sprintf("%s · %s needs at least one component", name, sh.Assessment), "subject", name, "assessment", sh.Assessment)
			continue
		}
		keys := map[string]bool{}
		var full calc.Marks
		ok := true
		for k, c := range sh.Components {
			cp := fmt.Sprintf("%s.components.%d", sp, k)
			if !keyPattern.MatchString(c.Key) {
				v.add("component_key_invalid", cp+".key", "a component key is 1–16 capital letters, digits or _ starting with a letter", "subject", name, "assessment", sh.Assessment, "component", c.Key)
			} else if keys[c.Key] {
				v.add("component_duplicate", cp+".key", fmt.Sprintf("%s · %s has two %s components", name, sh.Assessment, c.Key), "subject", name, "assessment", sh.Assessment, "component", c.Key)
			}
			keys[c.Key] = true
			cf, good := v.marks(c.Full, cp+".full", "full marks")
			if good && cf <= 0 {
				v.add("component_full_invalid", cp+".full", fmt.Sprintf("%s · %s: %s full marks must be above 0", name, sh.Assessment, c.Key), "subject", name, "assessment", sh.Assessment, "component", c.Key)
				good = false
			}
			ok = ok && good
			full += cf
			if c.Pass != nil {
				cpass, pgood := v.marks(*c.Pass, cp+".pass", "pass marks")
				if pgood && good && cpass > cf {
					v.add("component_pass_invalid", cp+".pass", fmt.Sprintf("%s · %s: %s pass marks cannot exceed its full marks", name, sh.Assessment, c.Key), "subject", name, "assessment", sh.Assessment, "component", c.Key)
				}
			}
		}
		if !ok {
			continue
		}
		if full > maxSheetFull {
			v.add("sheet_full_too_large", sp, fmt.Sprintf("%s · %s: full marks %s exceed 1000", name, sh.Assessment, full), "subject", name, "assessment", sh.Assessment, "full", full.String())
		}
		if pass, good := v.marks(sh.Pass, sp+".pass", "pass marks"); good && pass > full {
			v.add("sheet_pass_invalid", sp+".pass", fmt.Sprintf("%s · %s: pass marks %s exceed full marks %s", name, sh.Assessment, pass, full), "subject", name, "assessment", sh.Assessment, "pass", pass.String(), "full", full.String())
		}
	}
}

func (v *validator) terms(d Definition, s SubjectRule, path, name string) {
	// Terms the subject has sheets in, and which sheets each contribution uses.
	sheetTerms := map[string]bool{}
	used := map[string]bool{}
	for _, sh := range s.Sheets {
		if a, ok := d.Assessment(sh.Assessment); ok {
			sheetTerms[a.TermID] = true
		}
	}
	ruled := map[string]bool{}
	for j, tr := range s.Terms {
		tp := fmt.Sprintf("%s.terms.%d", path, j)
		term := v.termName(tr.TermID)
		if _, ok := v.ctx.Terms[tr.TermID]; !ok {
			v.add("term_rule_unknown_term", tp, fmt.Sprintf("%s: a term rule names a term this session does not have", name), "subject", name)
			continue
		}
		if ruled[tr.TermID] {
			v.add("term_rule_duplicate", tp, fmt.Sprintf("%s has two rules for %s", name, term), "subject", name, "term", term)
			continue
		}
		ruled[tr.TermID] = true
		full, fok := v.marks(tr.Full, tp+".full", "term full marks")
		if fok && (full <= 0 || full > maxSheetFull) {
			v.add("term_full_invalid", tp+".full", fmt.Sprintf("%s · %s: term full marks must be above 0 and at most 1000", name, term), "subject", name, "term", term)
			fok = false
		}
		if pass, ok := v.marks(tr.Pass, tp+".pass", "term pass marks"); ok && fok && pass > full {
			v.add("term_pass_invalid", tp+".pass", fmt.Sprintf("%s · %s: pass marks cannot exceed the term's full marks", name, term), "subject", name, "term", term)
		}
		if len(tr.Contributions) == 0 {
			v.add("no_contributions", tp+".contributions", fmt.Sprintf("%s · %s: say how the term mark is made", name, term), "subject", name, "term", term)
			continue
		}
		var worth calc.Marks
		worthOK := true
		sources := map[string]bool{}
		for k, c := range tr.Contributions {
			cp := fmt.Sprintf("%s.contributions.%d", tp, k)
			if !combines[c.Combine] {
				v.add("combine_invalid", cp+".combine", fmt.Sprintf("%s · %s: choose how the assessments combine", name, term), "subject", name, "term", term)
			}
			if len(c.From) == 0 {
				v.add("contribution_empty", cp+".from", fmt.Sprintf("%s · %s: a contribution needs at least one assessment", name, term), "subject", name, "term", term)
			}
			if c.Combine == "single" && len(c.From) != 1 {
				v.add("single_needs_one", cp+".from", fmt.Sprintf("%s · %s: a single contribution draws from exactly one assessment", name, term), "subject", name, "term", term)
			}
			if c.Combine == "best" && (c.Best < 1 || c.Best > len(c.From)) {
				v.add("best_invalid", cp+".best", fmt.Sprintf("%s · %s: best %d of %d is not possible", name, term, c.Best, len(c.From)), "subject", name, "term", term, "best", fmt.Sprint(c.Best), "of", fmt.Sprint(len(c.From)))
			}
			if c.Combine != "best" && c.Best != 0 {
				v.add("best_unexpected", cp+".best", fmt.Sprintf("%s · %s: only a best-of contribution takes N", name, term), "subject", name, "term", term)
			}
			for _, key := range c.From {
				a, ok := d.Assessment(key)
				switch {
				case !ok:
					v.add("source_unknown", cp+".from", fmt.Sprintf("%s · %s: %s is not an assessment", name, term, key), "subject", name, "term", term, "assessment", key)
				case a.TermID != tr.TermID:
					v.add("source_other_term", cp+".from", fmt.Sprintf("%s · %s: %s belongs to another term", name, term, key), "subject", name, "term", term, "assessment", key)
				case sources[key]:
					v.add("source_reused", cp+".from", fmt.Sprintf("%s · %s: %s is used twice", name, term, key), "subject", name, "term", term, "assessment", key)
				default:
					if _, has := s.Sheet(key); !has {
						v.add("source_no_sheet", cp+".from", fmt.Sprintf("%s · %s: %s has no %s sheet", name, term, key, name), "subject", name, "term", term, "assessment", key)
					}
				}
				sources[key] = true
				used[key] = true
			}
			w, ok := v.marks(c.Worth, cp+".worth", "worth")
			if ok && w <= 0 {
				v.add("worth_invalid", cp+".worth", fmt.Sprintf("%s · %s: each contribution must be worth more than 0", name, term), "subject", name, "term", term)
				ok = false
			}
			worthOK = worthOK && ok
			worth += w
		}
		if worthOK && fok && worth != full {
			v.add("worth_sum", tp+".contributions", fmt.Sprintf("%s · %s: contributions add up to %s, expected %s", name, term, worth, full), "subject", name, "term", term, "sum", worth.String(), "full", full.String())
		}
	}
	for id := range sheetTerms {
		if !ruled[id] {
			v.add("term_rule_missing", path+".terms", fmt.Sprintf("%s has marks in %s but no rule for its term mark", name, v.termName(id)), "subject", name, "term", v.termName(id))
		}
	}
	for _, sh := range s.Sheets {
		if _, ok := d.Assessment(sh.Assessment); ok && !used[sh.Assessment] {
			v.add("sheet_unused", path+".sheets", fmt.Sprintf("%s · %s: these marks are not used by any term rule", name, sh.Assessment), "subject", name, "assessment", sh.Assessment)
		}
	}
}

func (v *validator) combined(d Definition) {
	groups := map[string][]SubjectRule{}
	order := []string{}
	for i, s := range d.Subjects {
		if s.CombinedKey == "" {
			continue
		}
		if !keyPattern.MatchString(s.CombinedKey) {
			v.add("combined_key_invalid", fmt.Sprintf("subjects.%d.combined_key", i), "a combined key is 1–16 capital letters, digits or _ starting with a letter", "subject", v.subjectName(s.SubjectID))
			continue
		}
		if _, ok := groups[s.CombinedKey]; !ok {
			order = append(order, s.CombinedKey)
		}
		groups[s.CombinedKey] = append(groups[s.CombinedKey], s)
	}
	for _, key := range order {
		papers := groups[key]
		if len(papers) < 2 {
			v.add("combined_single", "subjects", fmt.Sprintf("%s combines only one subject; combine at least two papers", key), "key", key)
			continue
		}
		for _, p := range papers[1:] {
			if p.Role != papers[0].Role {
				v.add("combined_role_mismatch", "subjects", fmt.Sprintf("the papers of %s must have the same role", key), "key", key)
				break
			}
		}
		if papers[0].Role == "non_graded" {
			v.add("combined_non_graded", "subjects", fmt.Sprintf("the papers of %s are not graded, so they cannot be combined", key), "key", key)
		}
		// Papers combine per term: each paper needs a rule in the same terms.
		terms := func(s SubjectRule) string {
			ids := []string{}
			for _, t := range s.Terms {
				ids = append(ids, t.TermID)
			}
			sort.Strings(ids)
			return strings.Join(ids, ",")
		}
		for _, p := range papers[1:] {
			if terms(p) != terms(papers[0]) {
				v.add("combined_terms_mismatch", "subjects", fmt.Sprintf("the papers of %s must have term rules for the same terms", key), "key", key)
				break
			}
		}
	}
	if d.PaperRule != "combined" && d.PaperRule != "each_paper" {
		v.add("paper_rule_invalid", "paper_rule", "choose how combined papers pass")
	}
}

func (v *validator) annual(d Definition) {
	switch d.Annual.Method {
	case "final_only", "equal":
		if len(d.Annual.Weights) > 0 {
			v.add("annual_weights_unexpected", "annual.weights", "weights apply only to the weighted method")
		}
	case "weighted":
		terms := map[string]bool{}
		for _, a := range d.Assessments {
			terms[a.TermID] = true
		}
		total := 0
		for id, w := range d.Annual.Weights {
			if _, ok := v.ctx.Terms[id]; !ok || !terms[id] {
				v.add("annual_weight_unknown_term", "annual.weights", "a weight names a term the blueprint does not assess", "term", v.termName(id))
			}
			if w < 0 || w > 100 {
				v.add("annual_weight_invalid", "annual.weights", "each weight is between 0 and 100", "term", v.termName(id))
			}
			total += w
		}
		for id := range terms {
			if _, ok := d.Annual.Weights[id]; !ok {
				v.add("annual_weight_missing", "annual.weights", fmt.Sprintf("give %s a weight", v.termName(id)), "term", v.termName(id))
			}
		}
		if total != 100 {
			v.add("annual_weights_sum", "annual.weights", fmt.Sprintf("annual weights add up to %d, expected 100", total), "sum", fmt.Sprint(total))
		}
	default:
		v.add("annual_method_invalid", "annual.method", "choose final only, equal or weighted")
	}
}

func (v *validator) policy(d Definition) {
	p := d.Policy
	if p.GPARounding != "half_up" && p.GPARounding != "truncate" {
		v.add("policy_invalid", "policy.gpa_rounding", "choose how the GPA is rounded", "field", "gpa_rounding")
	}
	if p.Absent != "zero" && p.Absent != "missing" {
		v.add("policy_invalid", "policy.absent", "choose what an absence means", "field", "absent")
	}
	if p.Ranking != "competition" && p.Ranking != "dense" {
		v.add("policy_invalid", "policy.ranking", "choose how tied students are ranked", "field", "ranking")
	}
}

// bands: the school's grading scheme covers 0–100 without gaps or overlaps,
// and exactly one band has a grade point of 0.
func (v *validator) bands() {
	bands := append([]calc.Band(nil), v.ctx.Bands...)
	sort.Slice(bands, func(i, j int) bool { return bands[i].Min < bands[j].Min })
	zero := 0
	next := 0
	ok := len(bands) > 0
	for _, b := range bands {
		if b.Point == 0 {
			zero++
		}
		if b.Min != next || b.Max < b.Min {
			ok = false
		}
		next = b.Max + 1
	}
	if !ok || next != 101 || zero != 1 {
		v.add("bands_invalid", "grading", "the school's grading bands must cover 0–100 without gaps or overlaps, with exactly one failing band")
	}
}
