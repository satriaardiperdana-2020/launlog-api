package timezone

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		want    string
		invalid bool
	}{
		{"omitted or null", nil, Default, false},
	}
	for _, value := range []string{"", " \t\n ", "Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura", " Asia/Makassar ", "UTC", "America/New_York", "not/a-zone", "Local", "+08:00"} {
		input := value
		want := value
		invalid := false
		switch value {
		case "", " \t\n ":
			want = Default
		case " Asia/Makassar ":
			want = "Asia/Makassar"
		case "not/a-zone", "Local", "+08:00":
			want = ""
			invalid = true
		}
		tests = append(tests, struct {
			name    string
			input   *string
			want    string
			invalid bool
		}{value, &input, want, invalid})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.input)
			if got != tt.want || (err != nil) != tt.invalid {
				t.Fatalf("Normalize = %q, %v; want %q invalid=%t", got, err, tt.want, tt.invalid)
			}
		})
	}
}
