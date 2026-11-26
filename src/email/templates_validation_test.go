package email

import "testing"

func TestParseTemplateRequiresSubjectSeparatorAndBody(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr string
	}{
		{name: "valid", text: "Subject: Hello\n---\nBody"},
		{name: "missing separator", text: "Subject: Hello\nBody", wantErr: "invalid template syntax at line 1"},
		{name: "empty subject", text: "Subject:\n---\nBody", wantErr: "subject cannot be empty"},
		{name: "empty body", text: "Subject: Hello\n---\n", wantErr: "body cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseTemplate(tt.text)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("parseTemplate() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("parseTemplate() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestClosestTemplateVariable(t *testing.T) {
	known := map[string]string{"fqdn": "", "app_name": "", "timestamp": ""}
	if got := closestTemplateVariable("fqdnn", known); got != "fqdn" {
		t.Fatalf("closestTemplateVariable() = %q, want fqdn", got)
	}
	if got := closestTemplateVariable("unrelated", known); got != "" {
		t.Fatalf("closestTemplateVariable() = %q, want no suggestion", got)
	}
}
