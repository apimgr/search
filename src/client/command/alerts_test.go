package cmd

import "testing"

func TestAlertCreateDefaultsRespectExplicitDeliveryChannels(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantEmail   bool
		wantRSS     bool
		wantDefault string
	}{
		{name: "email defaults to email delivery", args: []string{"email=user@example.test"}, wantEmail: true, wantDefault: "daily"},
		{name: "RSS selection does not enable email", args: []string{"email=user@example.test", "deliver_rss=true"}, wantRSS: true, wantDefault: "daily"},
		{name: "explicit email opt-out is preserved", args: []string{"email=user@example.test", "deliver_email=false", "deliver_rss=true"}, wantRSS: true, wantDefault: "daily"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, err := parseAlertFields(test.args)
			if err != nil {
				t.Fatalf("parseAlertFields() error = %v", err)
			}
			fields.applyCreateDefaults()
			if fields.DeliverEmail != test.wantEmail || fields.DeliverRSS != test.wantRSS {
				t.Errorf("delivery channels = (email %t, RSS %t), want (email %t, RSS %t)", fields.DeliverEmail, fields.DeliverRSS, test.wantEmail, test.wantRSS)
			}
			if fields.Frequency != test.wantDefault {
				t.Errorf("frequency = %q, want %q", fields.Frequency, test.wantDefault)
			}
		})
	}
}
