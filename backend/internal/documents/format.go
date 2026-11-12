package documents

import (
	"strconv"
	"strings"
	"time"
)

// Everything a document shows is formatted here, in its language: Bangla
// digits, dates and labels. The Typst templates only lay text out.

var bnDigits = strings.NewReplacer("0", "০", "1", "১", "2", "২", "3", "৩", "4", "৪", "5", "৫", "6", "৬", "7", "৭", "8", "৮", "9", "৯")

// num writes the digits of s in the document's language.
func (l lang) num(s string) string {
	if l == "bn" {
		return bnDigits.Replace(s)
	}
	return s
}

func (l lang) int(n int) string { return l.num(strconv.Itoa(n)) }

// bnOrdinals are the usual Bangla ordinal words up to tenth; later positions
// take the numeral plus "তম" (১১তম).
var bnOrdinals = map[int]string{1: "১ম", 2: "২য়", 3: "৩য়", 4: "৪র্থ", 5: "৫ম", 6: "৬ষ্ঠ", 7: "৭ম", 8: "৮ম", 9: "৯ম", 10: "১০ম"}

// ordinal is a position in words: "3rd", in Bangla "৩য়".
func (l lang) ordinal(n int) string {
	if l == "bn" {
		if s, ok := bnOrdinals[n]; ok {
			return s
		}
		return l.int(n) + "তম"
	}
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}

// code writes an identifier (subject code, student ID): Bangla digits only
// when it is all digits ("১০১"); a code with letters ("BAN-101", "DEMO-001")
// stays as written, as it is printed elsewhere.
func (l lang) code(s string) string {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return s
	}
	return l.num(s)
}

// marks trims a two-decimal mark: "45.00" → "45", "45.50" → "45.5".
func (l lang) marks(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	v := *s
	if strings.Contains(v, ".") {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	return l.num(v)
}

// points keeps two decimals (GPA and grade points are always shown so).
func (l lang) points(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return l.num(*s)
}

var bnMonths = []string{"জানুয়ারি", "ফেব্রুয়ারি", "মার্চ", "এপ্রিল", "মে", "জুন", "জুলাই", "আগস্ট", "সেপ্টেম্বর", "অক্টোবর", "নভেম্বর", "ডিসেম্বর"}

// date formats an RFC 3339 instant as a day in Bangladesh time.
func (l lang) date(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	t = t.In(dhaka)
	if l == "bn" {
		return l.int(t.Day()) + " " + bnMonths[t.Month()-1] + " " + l.int(t.Year())
	}
	return t.Format("2 January 2006")
}

var dhaka = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Dhaka"); err == nil {
		return loc
	}
	return time.FixedZone("BST", 6*60*60)
}()

type lang string

// pick returns the Bangla text when the document is Bangla and it is set.
func (l lang) pick(en, bn string) string {
	if l == "bn" && strings.TrimSpace(bn) != "" {
		return bn
	}
	return en
}

// other is the name in the other language, shown under the main one.
func (l lang) other(en, bn string) string {
	if l == "bn" {
		if strings.TrimSpace(bn) == "" {
			return ""
		}
		return en
	}
	return bn
}

// t is a document label in the document's language.
func (l lang) t(key string) string {
	if l == "bn" {
		if s, ok := labelsBN[key]; ok {
			return s
		}
	}
	return labelsEN[key]
}

// componentLabel names a sheet component (CQ, MCQ, PRAC, TOTAL …).
func (l lang) componentLabel(key string) string {
	if s := l.t("comp_" + strings.ToUpper(key)); s != "" {
		return s
	}
	return key
}

var labelsEN = map[string]string{
	"marksheet": "Academic Transcript", "provisional": "PROVISIONAL — NOT PUBLISHED", "annual": "Annual result", "term": "Term result",
	"name": "Name", "student_id": "Student ID", "class": "Class", "section": "Section", "roll": "Roll", "session": "Session", "exam": "Examination", "photo": "Photo",
	"code": "Code", "subject": "Subject", "ct": "CT", "total": "Total", "highest": "Highest", "grade": "Grade", "gp": "GP", "fourth": "4th subject",
	"total_marks": "Total marks", "gpa": "GPA", "gpa_without": "GPA without 4th subject", "letter": "Letter grade", "result": "Result",
	"class_pos": "Class Position", "section_pos": "Section Position", "published": "Published", "version": "Version",
	"passed": "Passed", "failed": "Failed", "withheld": "Withheld", "withheld_note": "This result is withheld. Please contact the school.",
	"sig_class_teacher": "Class teacher", "sig_head": "Head teacher", "sig_guardian": "Guardian", "sig_date": "Date",
	"father": "Father", "mother": "Mother", "dob": "Date of birth", "failed_count": "Failed subjects", "attendance": "Attendance",
	"attendance_value": "%s / %s days (%s%%)", "pos_of": "%s of %s", "comment": "Teacher's Comments", "full": "Full",
	"established": "Established-%s", "version_medium": "Version", "medium_bangla": "Bangla Medium", "medium_english": "English Medium",
	"shift": "Shift", "group": "Group", "classes_held": "Classes Held", "days_present": "Present", "days_absent": "Absent",
	"student_information": "Student Information", "grades_reference": "Grades Reference", "obtained_marks": "Obtained Marks", "full_marks": "Full Marks",
	"lg": "LG", "total_gpa": "Total / GPA", "fourth_short": "4th Subj.", "fourth_suffix": "(4th)", "signature": "Signature",
	"transcript_note": "“—” = the subject has no such component. The small grey number is that component's full marks. 4th subject: only the grade point above 2.00 is added to the GPA, up to 5.00.",
	"obtained":        "Obtained", "optional_heading": "Optional (4th) subject — only the grade point above 2.00 is added to the GPA; it never fails the result",
	"combined": "Combined", "summary_title": "Summary",
	"footer":             "Issued from the published result (version %s, %s). Scan the QR code to check that it is genuine. Document code: %s",
	"footer_noqr":        "Issued from the published result (version %s, %s). Document code: %s",
	"footer_provisional": "Provisional preview for staff. These results are not published and may change.",
	"exempt":             "Exempt", "incomplete": "Incomplete", "absent": "Absent", "not_taken": "—",
	"grading": "Grading", "marks_range": "Marks", "point": "Point",
	"comp_CQ": "CQ", "comp_MCQ": "MCQ", "comp_PRAC": "Prac.", "comp_PRACTICAL": "Prac.", "comp_TOTAL": "Exam", "comp_WRITTEN": "Written", "comp_ORAL": "Oral", "comp_VIVA": "Viva",
	// Class documents.
	"tabulation": "Tabulation sheet", "merit_list": "Merit list", "failed_list": "Failed subjects", "summary": "Result summary",
	"all_sections": "All sections", "students": "Students", "position": "Position", "subjects_failed": "Failed subjects", "no_failures": "No student failed a subject.",
	"appeared": "Results", "pass_rate": "Pass rate", "grade_distribution": "Grade distribution (by GPA)", "subject_distribution": "Grades by subject",
	"page": "Page", "generated": "Generated", "count": "Count", "share": "Share", "by_section": "By section", "overall": "Overall",
}

var labelsBN = map[string]string{
	"marksheet": "ফলাফল পত্র", "provisional": "অস্থায়ী — প্রকাশিত নয়", "annual": "বার্ষিক ফলাফল", "term": "সাময়িক ফলাফল",
	"name": "নাম", "student_id": "শিক্ষার্থী আইডি", "class": "শ্রেণি", "section": "শাখা", "roll": "রোল", "session": "শিক্ষাবর্ষ", "exam": "পরীক্ষা", "photo": "ছবি",
	"code": "কোড", "subject": "বিষয়", "ct": "শ্রেণি অভীক্ষা", "total": "মোট", "highest": "সর্বোচ্চ", "grade": "গ্রেড", "gp": "জিপি", "fourth": "৪র্থ বিষয়",
	"total_marks": "মোট নম্বর", "gpa": "জিপিএ", "gpa_without": "৪র্থ বিষয় ছাড়া জিপিএ", "letter": "লেটার গ্রেড", "result": "ফলাফল",
	"class_pos": "শ্রেণিতে অবস্থান", "section_pos": "শাখায় অবস্থান", "published": "প্রকাশ", "version": "সংস্করণ",
	"passed": "উত্তীর্ণ", "failed": "অকৃতকার্য", "withheld": "স্থগিত", "withheld_note": "এই ফলাফল স্থগিত রাখা হয়েছে। অনুগ্রহ করে বিদ্যালয়ে যোগাযোগ করুন।",
	"sig_class_teacher": "শ্রেণি শিক্ষক", "sig_head": "প্রধান শিক্ষক", "sig_guardian": "অভিভাবক", "sig_date": "তারিখ",
	"father": "পিতা", "mother": "মাতা", "dob": "জন্ম তারিখ", "failed_count": "অকৃতকার্য বিষয়", "attendance": "উপস্থিতি",
	"attendance_value": "%s / %s দিন (%s%%)", "pos_of": "%[2]s জনের মধ্যে %[1]s", "comment": "শ্রেণি শিক্ষকের মন্তব্য", "full": "পূর্ণমান",
	"established": "স্থাপিত-%s", "version_medium": "ভার্সন", "medium_bangla": "বাংলা মাধ্যম", "medium_english": "ইংরেজি মাধ্যম",
	"shift": "শিফট", "group": "বিভাগ", "classes_held": "মোট ক্লাস", "days_present": "উপস্থিত", "days_absent": "অনুপস্থিত",
	"student_information": "শিক্ষার্থীর তথ্য", "grades_reference": "গ্রেড নির্দেশিকা", "obtained_marks": "প্রাপ্ত নম্বর", "full_marks": "পূর্ণ নম্বর",
	"lg": "গ্রেড", "total_gpa": "মোট / জিপিএ", "fourth_short": "৪র্থ বিষয়", "fourth_suffix": "(৪র্থ)", "signature": "স্বাক্ষর",
	"transcript_note": "“—” = এই বিষয়ে ওই অংশ নেই। ছোট ধূসর সংখ্যা ওই অংশের পূর্ণ নম্বর। ৪র্থ বিষয়: ২.০০-এর বেশি গ্রেড পয়েন্ট জিপিএ-তে যোগ হয়, সর্বোচ্চ ৫.০০।",
	"obtained":        "প্রাপ্ত", "optional_heading": "ঐচ্ছিক (৪র্থ) বিষয় — শুধু ২.০০-এর বেশি গ্রেড পয়েন্ট জিপিএ-তে যোগ হয়; এতে অকৃতকার্য হলেও ফলাফল অকৃতকার্য হয় না",
	"combined": "সমন্বিত", "summary_title": "সারসংক্ষেপ",
	"footer":             "প্রকাশিত ফলাফল থেকে প্রস্তুত (সংস্করণ %s, %s)। আসল কিনা যাচাই করতে কিউআর কোড স্ক্যান করুন। ডকুমেন্ট কোড: %s",
	"footer_noqr":        "প্রকাশিত ফলাফল থেকে প্রস্তুত (সংস্করণ %s, %s)। ডকুমেন্ট কোড: %s",
	"footer_provisional": "শিক্ষক ও কর্মীদের জন্য অস্থায়ী নমুনা। এই ফলাফল প্রকাশিত নয় এবং পরিবর্তন হতে পারে।",
	"exempt":             "অব্যাহতি", "incomplete": "অসম্পূর্ণ", "absent": "অনুপস্থিত", "not_taken": "—",
	"grading": "গ্রেডিং", "marks_range": "নম্বর", "point": "পয়েন্ট",
	"comp_CQ": "সৃজনশীল", "comp_MCQ": "বহুনির্বাচনী", "comp_PRAC": "ব্যবহারিক", "comp_PRACTICAL": "ব্যবহারিক", "comp_TOTAL": "পরীক্ষা", "comp_WRITTEN": "লিখিত", "comp_ORAL": "মৌখিক", "comp_VIVA": "মৌখিক",
	"tabulation": "টেবুলেশন শিট", "merit_list": "মেধা তালিকা", "failed_list": "অকৃতকার্য বিষয়ের তালিকা", "summary": "ফলাফলের সারসংক্ষেপ",
	"all_sections": "সব শাখা", "students": "শিক্ষার্থী", "position": "অবস্থান", "subjects_failed": "অকৃতকার্য বিষয়", "no_failures": "কোনো শিক্ষার্থী কোনো বিষয়ে অকৃতকার্য হয়নি।",
	"appeared": "ফলাফল", "pass_rate": "পাসের হার", "grade_distribution": "গ্রেড বণ্টন (জিপিএ অনুযায়ী)", "subject_distribution": "বিষয়ভিত্তিক গ্রেড",
	"page": "পৃষ্ঠা", "generated": "প্রস্তুত", "count": "সংখ্যা", "share": "হার", "by_section": "শাখাভিত্তিক", "overall": "সার্বিক",
}

// studentCode is what documents print as the student ID: the school's student
// ID (the sign-in ID, also on the ID card), or the admission number when the
// school gave none, as on the student's profile.
func studentCode(code, admissionNo string) string {
	if code = strings.TrimSpace(code); code != "" {
		return code
	}
	return admissionNo
}
