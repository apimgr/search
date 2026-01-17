package config

import "testing"

func TestParseBool(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal bool
		want       bool
		wantErr    bool
	}{
		// True values
		{"true lowercase", "true", false, true, false},
		{"TRUE uppercase", "TRUE", false, true, false},
		{"True mixed", "True", false, true, false},
		{"yes lowercase", "yes", false, true, false},
		{"YES uppercase", "YES", false, true, false},
		{"1", "1", false, true, false},
		{"on lowercase", "on", false, true, false},
		{"ON uppercase", "ON", false, true, false},
		{"enabled", "enabled", false, true, false},
		{"ENABLED", "ENABLED", false, true, false},

		// False values
		{"false lowercase", "false", true, false, false},
		{"FALSE uppercase", "FALSE", true, false, false},
		{"no lowercase", "no", true, false, false},
		{"NO uppercase", "NO", true, false, false},
		{"0", "0", true, false, false},
		{"off lowercase", "off", true, false, false},
		{"OFF uppercase", "OFF", true, false, false},
		{"disabled", "disabled", true, false, false},
		{"DISABLED", "DISABLED", true, false, false},

		// Empty/invalid returns default
		{"empty string default true", "", true, true, false},
		{"empty string default false", "", false, false, false},
		{"whitespace only", "   ", false, false, false},

		// Invalid values return error
		{"invalid value", "maybe", false, false, true},
		{"invalid number", "2", false, false, true},
		{"random string", "xyz", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBool(tt.input, tt.defaultVal)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBool(%q, %v) error = %v, wantErr %v", tt.input, tt.defaultVal, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseBool(%q, %v) = %v, want %v", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestMustParseBool(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal bool
		want       bool
	}{
		{"true", "true", false, true},
		{"false", "false", true, false},
		{"empty returns default", "", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MustParseBool(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("MustParseBool(%q, %v) = %v, want %v", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestMustParseBoolPanicsOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseBool should panic on invalid value")
		}
	}()
	MustParseBool("invalid", false)
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"yes", "yes", true},
		{"1", "1", true},
		{"on", "on", true},
		{"enabled", "enabled", true},
		{"false", "false", false},
		{"no", "no", false},
		{"0", "0", false},
		{"empty", "", false},
		{"invalid", "maybe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTruthy(tt.input)
			if got != tt.want {
				t.Errorf("IsTruthy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsFalsy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"false", "false", true},
		{"FALSE", "FALSE", true},
		{"no", "no", true},
		{"0", "0", true},
		{"off", "off", true},
		{"disabled", "disabled", true},
		{"true", "true", false},
		{"yes", "yes", false},
		{"1", "1", false},
		{"empty", "", false},
		{"invalid", "maybe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFalsy(tt.input)
			if got != tt.want {
				t.Errorf("IsFalsy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
