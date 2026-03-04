package precomputefeatures

import (
	"testing"
)

func TestParseAsOf(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantHas bool
		wantYMD string
		wantErr bool
	}{
		{name: "empty -> no date", in: "", wantHas: false, wantYMD: "", wantErr: false},
		{name: "whitespace -> no date", in: "  ", wantHas: false, wantYMD: "", wantErr: false},
		{name: "valid date", in: "2024-02-03", wantHas: true, wantYMD: "2024-02-03", wantErr: false},
		{name: "invalid date", in: "2024-13-40", wantHas: false, wantYMD: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, has, err := parseAsOf(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if has != tt.wantHas {
				t.Fatalf("has mismatch: got %v want %v", has, tt.wantHas)
			}
			if tt.wantHas {
				if got.Format("2006-01-02") != tt.wantYMD {
					t.Fatalf("date mismatch: got %s want %s", got.Format("2006-01-02"), tt.wantYMD)
				}
			} else {
				if !got.IsZero() {
					t.Fatalf("expected zero time when no date, got %v", got)
				}
			}
		})
	}
}

func TestValidateAlpha(t *testing.T) {
	cases := []struct {
		name string
		v    float64
		ok   bool
	}{
		{"too small", 0.0, false},
		{"negative", -0.1, false},
		{"upper bound ok", 1.0, true},
		{"typical", 0.3, true},
		{"too large", 1.1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAlpha(c.v)
			if (err == nil) != c.ok {
				t.Fatalf("validateAlpha(%v) ok=%v err=%v", c.v, c.ok, err)
			}
		})
	}
}

func TestValidateLastN(t *testing.T) {
	tests := []struct {
		name string
		n    int
		ok   bool
	}{
		{"negative", -1, false},
		{"zero", 0, true},
		{"positive", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLastN(tt.n)
			if (err == nil) != tt.ok {
				t.Fatalf("validateLastN(%d) ok=%v err=%v", tt.n, tt.ok, err)
			}
		})
	}
}
