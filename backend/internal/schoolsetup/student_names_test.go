package schoolsetup

import (
	"strings"
	"testing"
)

func TestSyntheticStudentNames(t *testing.T) {
	seen := map[string]bool{}
	counts := map[string]int{}
	for i := 0; i < 180; i++ {
		en, bn, gender := syntheticStudentName(i)
		if !strings.HasPrefix(en, "TEST ") || !strings.HasPrefix(bn, "টেস্ট ") || !strings.Contains(bn, " ") || seen[en] || (gender != "male" && gender != "female") {
			t.Fatalf("invalid or repeated paired TEST name at %d: %q %q %q", i, en, bn, gender)
		}
		seen[en] = true
		counts[gender]++
		otherEN, otherBN, otherGender := syntheticStudentName(i)
		if en != otherEN || bn != otherBN || gender != otherGender {
			t.Fatal("name generation must be stable")
		}
	}
	if counts["male"] != 90 || counts["female"] != 90 {
		t.Fatalf("gender balance: %+v", counts)
	}
	if en, bn, gender := syntheticStudentName(0); en != "TEST Arif Rahman" || bn != "টেস্ট আরিফ রহমান" || gender != "male" {
		t.Fatal("first bilingual name pair changed")
	}
}
