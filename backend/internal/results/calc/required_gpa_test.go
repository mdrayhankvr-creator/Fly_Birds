package calc

import "testing"

func TestRequiredGPA(t *testing.T) {
	cases := []struct {
		points []Points
		passed bool
		policy Policy
		want   string
	}{
		{[]Points{500, 400, 350}, true, Policy{}, "4.17"},              // 12.50 / 3 = 4.1666… half up
		{[]Points{500, 400, 350}, true, Policy{GPA: Truncate}, "4.16"}, // truncated
		{[]Points{500, 500}, true, Policy{}, "5.00"},
		{[]Points{500, 0}, false, Policy{}, "0.00"},
	}
	for _, c := range cases {
		got, err := RequiredGPA(c.points, c.passed, c.policy)
		if err != nil || got.String() != c.want {
			t.Fatalf("RequiredGPA(%v, %v) = %s, %v; want %s", c.points, c.passed, got, err, c.want)
		}
	}
	if _, err := RequiredGPA(nil, true, Policy{}); err == nil {
		t.Fatal("no subjects must be invalid")
	}
}
