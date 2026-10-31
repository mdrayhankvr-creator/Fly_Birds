package people

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// xlsxWith builds a small workbook whose first sheet uses shared strings, as
// spreadsheet programs write them.
func xlsxWith(t *testing.T, rows [][]string) []byte {
	t.Helper()
	var shared []string
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		sheet.WriteString(`<row>`)
		for c, v := range row {
			ref := columnName(c)
			if v == "" {
				continue
			}
			if n := len(v); n > 0 && strings.Trim(v, "0123456789.") == "" {
				sheet.WriteString(`<c r="` + ref + itoa(r+1) + `"><v>` + v + `</v></c>`)
				continue
			}
			shared = append(shared, v)
			sheet.WriteString(`<c r="` + ref + itoa(r+1) + `" t="s"><v>` + itoa(len(shared)-1) + `</v></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range shared {
		sst.WriteString(`<si><t>` + xmlText(s) + `</t></si>`)
	}
	sst.WriteString(`</sst>`)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"xl/workbook.xml":            `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Data" sheetId="1" r:id="rId7"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId7" Type="worksheet" Target="worksheets/data.xml"/></Relationships>`,
		"xl/worksheets/data.xml":     sheet.String(),
		"xl/sharedStrings.xml":       sst.String(),
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

func TestReadSheetCSVKeepsBangla(t *testing.T) {
	csv := "\xef\xbb\xbfadmission_no,name_en,name_bn\n# a note row\nA-1,Rahim,রহিম উদ্দিন\n,,\n\"A-2\",\"Karim, Jr\",করিম\n"
	rows, err := ReadSheet([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0] != "admission_no" || rows[1][2] != "রহিম উদ্দিন" || rows[2][1] != "Karim, Jr" {
		t.Fatalf("rows %q", rows)
	}
	semi, err := ReadSheet([]byte("admission_no;name_en\nA-3;Selina\n"))
	if err != nil || len(semi) != 2 || semi[1][1] != "Selina" {
		t.Fatalf("semicolon CSV %q %v", semi, err)
	}
	// A CSV saved in a legacy code page would garble Bangla: refused.
	if _, err := ReadSheet([]byte("admission_no,name_bn\nA-1,\xe0\x41\n")); !errors.Is(err, ErrSheetNotUTF8) {
		t.Fatalf("non-UTF-8: %v", err)
	}
}

func TestReadSheetXLSX(t *testing.T) {
	file := xlsxWith(t, [][]string{{"admission_no", "name_en", "name_bn", "roll"}, {"X-1", "Nadia", "নাদিয়া", "12"}, {"", "", "", ""}, {"X-2", "Tanvir", "তানভীর", ""}})
	rows, err := ReadSheet(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1][2] != "নাদিয়া" || rows[1][3] != "12" || rows[2][0] != "X-2" {
		t.Fatalf("rows %q", rows)
	}
	if _, err := ReadSheet([]byte("PK\x03\x04 not really a zip")); !errors.Is(err, ErrSheetFormat) {
		t.Fatalf("broken xlsx: %v", err)
	}
}

func TestTemplatesReadBackTheirColumns(t *testing.T) {
	xlsx, err := TemplateXLSX()
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string][]byte{"xlsx": xlsx, "csv": TemplateCSV()} {
		rows, err := ReadSheet(file)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(rows) != 1 || len(rows[0]) != len(BulkColumns) {
			t.Fatalf("%s template rows %q", name, rows)
		}
		for i, c := range BulkColumns {
			if headerKey(rows[0][i]) != c.Key {
				t.Fatalf("%s column %d is %q", name, i, rows[0][i])
			}
		}
	}
	if !bytes.Contains(TemplateCSV(), []byte("ভর্তি নম্বর")) {
		t.Fatal("the CSV template has no Bangla guidance")
	}
}

func TestParseBulkDate(t *testing.T) {
	for in, want := range map[string]string{"2014-03-21": "2014-03-21", "21/03/2014": "2014-03-21", "1/2/2015": "2015-02-01", "21-03-2014": "2014-03-21", "41719": "2014-03-21", "": ""} {
		if got, ok := parseBulkDate(in); !ok || got != want {
			t.Errorf("%q: got %q %v want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"2014-13-01", "yesterday", "32/01/2014"} {
		if _, ok := parseBulkDate(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if columnIndex("C7") != 2 || columnIndex("AA1") != 26 || columnName(26) != "AA" {
		t.Fatal("column letters")
	}
}
