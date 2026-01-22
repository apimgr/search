package config

import (
	"strings"
	"testing"
)

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

// TestParseBoolExtendedTruthy tests all truthy values defined in bool.go
func TestParseBoolExtendedTruthy(t *testing.T) {
	truthyValues := []string{
		"1", "y", "t",
		"yes", "true", "on", "ok",
		"enable", "enabled",
		"yep", "yup", "yeah",
		"aye", "si", "oui", "da", "hai",
		"affirmative", "accept", "allow", "grant",
		"sure", "totally",
	}

	for _, val := range truthyValues {
		t.Run(val, func(t *testing.T) {
			result, err := ParseBool(val, false)
			if err != nil {
				t.Errorf("ParseBool(%q) error = %v", val, err)
			}
			if !result {
				t.Errorf("ParseBool(%q) = false, want true", val)
			}

			// Also test uppercase
			result, err = ParseBool(strings.ToUpper(val), false)
			if err != nil {
				t.Errorf("ParseBool(%q) error = %v", strings.ToUpper(val), err)
			}
			if !result {
				t.Errorf("ParseBool(%q) = false, want true", strings.ToUpper(val))
			}
		})
	}
}

// TestParseBoolExtendedFalsy tests all falsy values defined in bool.go
func TestParseBoolExtendedFalsy(t *testing.T) {
	falsyValues := []string{
		"0", "n", "f",
		"no", "false", "off",
		"disable", "disabled",
		"nope", "nah", "nay",
		"nein", "non", "niet", "iie", "lie",
		"negative", "reject", "block", "revoke",
		"deny", "never", "noway",
	}

	for _, val := range falsyValues {
		t.Run(val, func(t *testing.T) {
			result, err := ParseBool(val, true) // default true to verify it returns false
			if err != nil {
				t.Errorf("ParseBool(%q) error = %v", val, err)
			}
			if result {
				t.Errorf("ParseBool(%q) = true, want false", val)
			}

			// Also test uppercase
			result, err = ParseBool(strings.ToUpper(val), true)
			if err != nil {
				t.Errorf("ParseBool(%q) error = %v", strings.ToUpper(val), err)
			}
			if result {
				t.Errorf("ParseBool(%q) = true, want false", strings.ToUpper(val))
			}
		})
	}
}

// TestIsTruthyExtended tests all truthy values
func TestIsTruthyExtended(t *testing.T) {
	truthyValues := []string{
		"1", "y", "t", "yes", "true", "on", "ok",
		"enable", "enabled", "yep", "yup", "yeah",
		"aye", "si", "oui", "da", "hai",
		"affirmative", "accept", "allow", "grant",
		"sure", "totally",
	}

	for _, val := range truthyValues {
		if !IsTruthy(val) {
			t.Errorf("IsTruthy(%q) = false, want true", val)
		}
		if !IsTruthy(strings.ToUpper(val)) {
			t.Errorf("IsTruthy(%q) = false, want true", strings.ToUpper(val))
		}
	}
}

// TestIsFalsyExtended tests all falsy values
func TestIsFalsyExtended(t *testing.T) {
	falsyValues := []string{
		"0", "n", "f", "no", "false", "off",
		"disable", "disabled", "nope", "nah", "nay",
		"nein", "non", "niet", "iie", "lie",
		"negative", "reject", "block", "revoke",
		"deny", "never", "noway",
	}

	for _, val := range falsyValues {
		if !IsFalsy(val) {
			t.Errorf("IsFalsy(%q) = false, want true", val)
		}
		if !IsFalsy(strings.ToUpper(val)) {
			t.Errorf("IsFalsy(%q) = false, want true", strings.ToUpper(val))
		}
	}
}

// TestParseBoolWithWhitespace tests whitespace handling
func TestParseBoolWithWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"  true  ", true},
		{"\ttrue\t", true},
		{"\nfalse\n", false},
		{"  yes  ", true},
		{"  no  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBool(tt.input, !tt.want)
			if err != nil {
				t.Errorf("ParseBool(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
