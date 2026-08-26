package evidence

import "testing"

func TestParseFixedBasic(t *testing.T) {
	cases := []struct {
		in        string
		precision int
		want      int64
	}{
		{"12.34", 2, 1234},
		{"12.3", 2, 1230},
		{"12", 2, 1200},
		{"0.5", 1, 5},
		{".5", 2, 50},
		{"1.", 2, 100},
		{"+7.25", 2, 725},
		{"-7.25", 2, -725},
		{"0", 0, 0},
		{"0", 5, 0},
	}
	for _, c := range cases {
		got, err := ParseFixed(c.in, c.precision)
		if err != nil {
			t.Errorf("ParseFixed(%q, %d) error: %v", c.in, c.precision, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseFixed(%q, %d) = %d, want %d", c.in, c.precision, got, c.want)
		}
	}
}

func TestParseFixedPrecisionExceeded(t *testing.T) {
	if _, err := ParseFixed("1.234", 2); err != ErrPrecision {
		t.Errorf("want ErrPrecision, got %v", err)
	}
}

func TestParseFixedInvalidPrecision(t *testing.T) {
	if _, err := ParseFixed("1.2", -1); err != ErrInvalidPrecision {
		t.Errorf("want ErrInvalidPrecision, got %v", err)
	}
	if _, err := ParseFixed("1.2", 19); err != ErrInvalidPrecision {
		t.Errorf("want ErrInvalidPrecision, got %v", err)
	}
}

func TestParseFixedInvalidFormat(t *testing.T) {
	for _, in := range []string{"", "abc", "1.2.3", "-", "+", "1e3", "1,2", "--1"} {
		if _, err := ParseFixed(in, 2); err == nil {
			t.Errorf("ParseFixed(%q, 2) should fail", in)
		}
	}
}

func TestParseFixedOverflow(t *testing.T) {
	if _, err := ParseFixed("9223372036854775808", 0); err != ErrOverflow {
		t.Errorf("want ErrOverflow, got %v", err)
	}
	if _, err := ParseFixed("92233720368547758.08", 2); err != ErrOverflow {
		t.Errorf("want ErrOverflow, got %v", err)
	}
	if _, err := ParseFixed("-9223372036854775809", 0); err != ErrOverflow {
		t.Errorf("want ErrOverflow, got %v", err)
	}
}

func TestParseFixedMaxInt64(t *testing.T) {
	got, err := ParseFixed("9223372036854775807", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9223372036854775807 {
		t.Errorf("got %d", got)
	}
}

func TestParseFixedLongText(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = '1'
	}
	if _, err := ParseFixed(string(long), 2); err != ErrInvalidLength {
		t.Errorf("want ErrInvalidLength, got %v", err)
	}
}
