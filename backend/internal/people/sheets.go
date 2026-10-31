package people

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Reading and writing the bulk admission spreadsheet (checkpoint 5). CSV
// must be UTF-8 (a byte-order mark is fine) so Bangla names survive; .xlsx
// is read with the standard library only. Both give rows of trimmed cells.

var (
	ErrSheetFormat  = errors.New("the file is not a readable CSV or .xlsx spreadsheet")
	ErrSheetNotUTF8 = errors.New("the CSV file is not UTF-8")
	ErrSheetTooBig  = errors.New("the spreadsheet has too many rows")
)

const (
	maxSheetPart = 16 << 20 // largest uncompressed .xlsx part read
	maxSheetRows = MaxBulkRows + 50
)

// ReadSheet reads a CSV or .xlsx file into rows of cells. Rows whose first
// cell starts with "#" are notes (the CSV template's guidance) and skipped,
// as are empty rows.
func ReadSheet(data []byte) ([][]string, error) {
	var rows [][]string
	var err error
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		rows, err = readXLSX(data)
	} else {
		rows, err = readCSV(data)
	}
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		empty := true
		for i := range row {
			row[i] = strings.TrimSpace(strings.ReplaceAll(row[i], " ", " "))
			if row[i] != "" {
				empty = false
			}
		}
		if empty || strings.HasPrefix(row[0], "#") {
			continue
		}
		out = append(out, row)
		if len(out) > maxSheetRows {
			return nil, ErrSheetTooBig
		}
	}
	return out, nil
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return nil, ErrSheetNotUTF8
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	// Spreadsheets set to some locales save with semicolons.
	if first, _, _ := strings.Cut(string(data), "\n"); !strings.Contains(first, ",") && strings.Contains(first, ";") {
		reader.Comma = ';'
	}
	var rows [][]string
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, ErrSheetFormat
		}
		rows = append(rows, row)
		if len(rows) > maxSheetRows*2 {
			return nil, ErrSheetTooBig
		}
	}
}

// readXLSX reads the first worksheet of an .xlsx workbook.
func readXLSX(data []byte) ([][]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) > 500 {
		return nil, ErrSheetFormat
	}
	part := func(name string) ([]byte, error) {
		for _, f := range archive.File {
			if f.Name == name {
				r, err := f.Open()
				if err != nil {
					return nil, ErrSheetFormat
				}
				defer r.Close()
				b, err := io.ReadAll(io.LimitReader(r, maxSheetPart+1))
				if err != nil || len(b) > maxSheetPart {
					return nil, ErrSheetFormat
				}
				return b, nil
			}
		}
		return nil, nil
	}
	sheetPath, err := firstSheetPath(part)
	if err != nil {
		return nil, err
	}
	var shared []string
	if b, err := part("xl/sharedStrings.xml"); err != nil {
		return nil, err
	} else if b != nil {
		var sst struct {
			Items []struct {
				T    string `xml:"t"`
				Runs []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &sst); err != nil {
			return nil, ErrSheetFormat
		}
		for _, si := range sst.Items {
			text := si.T
			for _, r := range si.Runs {
				text += r.T
			}
			shared = append(shared, text)
		}
	}
	b, err := part(sheetPath)
	if err != nil || b == nil {
		return nil, ErrSheetFormat
	}
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline struct {
					T    string `xml:"t"`
					Runs []struct {
						T string `xml:"t"`
					} `xml:"r"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(b, &sheet); err != nil {
		return nil, ErrSheetFormat
	}
	if len(sheet.Rows) > maxSheetRows*2 {
		return nil, ErrSheetTooBig
	}
	rows := make([][]string, 0, len(sheet.Rows))
	for _, r := range sheet.Rows {
		var row []string
		for i, c := range r.Cells {
			col := i
			if c.Ref != "" {
				if col = columnIndex(c.Ref); col < 0 || col > 200 {
					return nil, ErrSheetFormat
				}
			}
			var text string
			switch c.Type {
			case "s":
				n, err := strconv.Atoi(c.Value)
				if err != nil || n < 0 || n >= len(shared) {
					return nil, ErrSheetFormat
				}
				text = shared[n]
			case "inlineStr":
				text = c.Inline.T
				for _, run := range c.Inline.Runs {
					text += run.T
				}
			default:
				text = c.Value
			}
			for len(row) <= col {
				row = append(row, "")
			}
			row[col] = text
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func firstSheetPath(part func(string) ([]byte, error)) (string, error) {
	workbook, err := part("xl/workbook.xml")
	if err != nil || workbook == nil {
		return "", ErrSheetFormat
	}
	var wb struct {
		Sheets []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(workbook, &wb); err != nil || len(wb.Sheets) == 0 {
		return "", ErrSheetFormat
	}
	rels, err := part("xl/_rels/workbook.xml.rels")
	if err != nil || rels == nil {
		return "xl/worksheets/sheet1.xml", nil
	}
	var rs struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(rels, &rs); err != nil {
		return "", ErrSheetFormat
	}
	for _, r := range rs.Items {
		if r.ID == wb.Sheets[0].ID {
			target := strings.TrimPrefix(r.Target, "/")
			if !strings.HasPrefix(target, "xl/") {
				target = path.Join("xl", target)
			}
			return path.Clean(target), nil
		}
	}
	return "xl/worksheets/sheet1.xml", nil
}

// columnIndex turns a cell reference such as "C7" into a 0-based column.
func columnIndex(ref string) int {
	n := 0
	letters := 0
	for _, ch := range ref {
		if ch >= 'A' && ch <= 'Z' {
			n = n*26 + int(ch-'A'+1)
			letters++
			continue
		}
		break
	}
	if letters == 0 || letters > 3 {
		return -1
	}
	return n - 1
}

// BulkColumns are the template's columns, in order. Starred ones are required.
var BulkColumns = []struct{ Key, Guide string }{
	{"admission_no", "ভর্তি নম্বর — আবশ্যক, প্রতিটি শিক্ষার্থীর জন্য আলাদা (যেমন 2026-0145)"},
	{"name_en", "শিক্ষার্থীর নাম ইংরেজিতে — আবশ্যক"},
	{"name_bn", "শিক্ষার্থীর নাম বাংলায় — ঐচ্ছিক"},
	{"student_id", "স্টুডেন্ট আইডি — ঐচ্ছিক; থাকলে এটিই সাইন-ইন আইডি হয়"},
	{"class", "শ্রেণি — আবশ্যক, সফটওয়্যারে যে নামে আছে হুবহু (যেমন Class 6)"},
	{"section", "শাখা — আবশ্যক, ওই শ্রেণির শাখার নাম (যেমন A)"},
	{"roll", "রোল — ঐচ্ছিক, পূর্ণসংখ্যা; শাখায় আগে থেকে থাকলে চলবে না"},
	{"date_of_birth", "জন্মতারিখ — ঐচ্ছিক; 2014-03-21 বা 21/03/2014"},
	{"gender", "লিঙ্গ — ঐচ্ছিক: male, female বা other"},
	{"blood_group", "রক্তের গ্রুপ — ঐচ্ছিক: A+, A-, B+, B-, AB+, AB-, O+, O-"},
	{"admission_date", "ভর্তির তারিখ — ঐচ্ছিক; না দিলে আজকের তারিখ"},
	{"father_name_en", "বাবার নাম (ইংরেজি) — ঐচ্ছিক"},
	{"father_name_bn", "বাবার নাম (বাংলা) — ঐচ্ছিক"},
	{"mother_name_en", "মায়ের নাম (ইংরেজি) — ঐচ্ছিক"},
	{"mother_name_bn", "মায়ের নাম (বাংলা) — ঐচ্ছিক"},
	{"present_address", "বর্তমান ঠিকানা — ঐচ্ছিক"},
	{"permanent_address", "স্থায়ী ঠিকানা — ঐচ্ছিক"},
	{"guardian_name", "অভিভাবকের নাম (ইংরেজি) — অভিভাবক যোগ করতে চাইলে আবশ্যক"},
	{"guardian_name_bn", "অভিভাবকের নাম (বাংলা) — ঐচ্ছিক"},
	{"guardian_relationship", "সম্পর্ক — যেমন Father, Mother; না দিলে Guardian"},
	{"guardian_phone", "অভিভাবকের ফোন — ফোন বা ইমেইলের একটি আবশ্যক; একই ফোনের অভিভাবক আগে থেকে থাকলে তাঁকেই যুক্ত করা হবে"},
	{"guardian_email", "অভিভাবকের ইমেইল — ঐচ্ছিক"},
	{"login_email", "শিক্ষার্থীর লগইন ইমেইল — ঐচ্ছিক; ইমেইল ছাড়াও স্টুডেন্ট আইডি দিয়ে সাইন ইন করা যায়"},
}

// guidanceBN is the Bangla guidance in the template.
var guidanceBN = []string{
	"শিক্ষার্থী ভর্তির ফাইল — কীভাবে পূরণ করবেন",
	"১. প্রথম সারির কলামের নাম বদলাবেন না। প্রতিটি শিক্ষার্থীর জন্য একটি সারি লিখুন।",
	"২. একটি ফাইলে সর্বোচ্চ ১,০০০ জন শিক্ষার্থী।",
	"৩. বাংলা নাম ঠিক রাখতে CSV হিসেবে সংরক্ষণ করলে \"CSV UTF-8\" বেছে নিন, অথবা .xlsx ফাইলই আপলোড করুন।",
	"৪. আপলোডের পর প্রতিটি সারি দেখানো হবে; ভুল থাকলে কোন সারিতে কী ভুল তা লেখা থাকবে। সব ঠিক হলে তবেই ভর্তি হবে — হয় সবাই, নয়তো কেউ না।",
	"৫. একই ফাইল আবার আপলোড করলে আগে ভর্তি হওয়া শিক্ষার্থীদের বাদ দেওয়া হবে; কেউ দুবার ভর্তি হবে না।",
	"",
	"কলামগুলো:",
}

// TemplateXLSX builds the bulk admission template: a "Students" sheet with
// the column names, and a guidance sheet in Bangla.
func TemplateXLSX() ([]byte, error) {
	var header strings.Builder
	header.WriteString(`<row r="1">`)
	for i, c := range BulkColumns {
		fmt.Fprintf(&header, `<c r="%s1" t="inlineStr" s="1"><is><t>%s</t></is></c>`, columnName(i), xmlText(c.Key))
	}
	header.WriteString(`</row>`)
	var cols strings.Builder
	for i := range BulkColumns {
		fmt.Fprintf(&cols, `<col min="%d" max="%d" width="18" customWidth="1"/>`, i+1, i+1)
	}
	students := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols>` + cols.String() + `</cols><sheetData>` + header.String() + `</sheetData></worksheet>`
	var guide strings.Builder
	line := 1
	addRow := func(a, b string, bold bool) {
		style := ""
		if bold {
			style = ` s="1"`
		}
		fmt.Fprintf(&guide, `<row r="%d"><c r="A%d" t="inlineStr"%s><is><t>%s</t></is></c>`, line, line, style, xmlText(a))
		if b != "" {
			fmt.Fprintf(&guide, `<c r="B%d" t="inlineStr"><is><t>%s</t></is></c>`, line, xmlText(b))
		}
		guide.WriteString(`</row>`)
		line++
	}
	for i, g := range guidanceBN {
		addRow(g, "", i == 0)
	}
	for _, c := range BulkColumns {
		addRow(c.Key, c.Guide, false)
	}
	guidance := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><cols><col min="1" max="1" width="28" customWidth="1"/><col min="2" max="2" width="100" customWidth="1"/></cols><sheetData>` + guide.String() + `</sheetData></worksheet>`
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Students" sheetId="1" r:id="rId1"/><sheet name="নির্দেশনা (Guidance)" sheetId="2" r:id="rId2"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs></styleSheet>`},
		{"xl/worksheets/sheet1.xml", students},
		{"xl/worksheets/sheet2.xml", guidance},
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		part, err := w.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(part, f.body); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// TemplateCSV is the same template as UTF-8 CSV: the guidance as "#" lines
// (skipped on upload), then the column names.
func TemplateCSV() []byte {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	for _, g := range guidanceBN {
		if g != "" {
			_ = w.Write([]string{"# " + g})
		}
	}
	for _, c := range BulkColumns {
		_ = w.Write([]string{"# " + c.Key + ": " + c.Guide})
	}
	keys := make([]string, len(BulkColumns))
	for i, c := range BulkColumns {
		keys[i] = c.Key
	}
	_ = w.Write(keys)
	w.Flush()
	return b.Bytes()
}

func columnName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
