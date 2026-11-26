package server

import (
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/logging"
)

func configureAuditRetention(cfg *config.Config, audit *logging.AuditLogger) error {
	if cfg == nil || audit == nil {
		return nil
	}
	keep := cfg.Server.Logs.Audit.Keep
	if keep == "" {
		keep = cfg.Server.Logs.Defaults.Keep
	}
	policy, err := logging.AuditRetentionPolicyFromKeep(keep)
	if err != nil {
		return err
	}
	minimum := MinimumComplianceAuditRetention(cfg.Server.Compliance)
	if policy.MaxAge >= 0 && policy.MaxAge < minimum {
		policy.MaxAge = minimum
	}
	return audit.SetRetention(policy)
}

func MinimumComplianceAuditRetention(compliance config.ComplianceConfig) time.Duration {
	var minimum time.Duration
	if compliance.GDPR || compliance.CCPA || compliance.SOC2 || compliance.PCIDSS {
		minimum = 365 * 24 * time.Hour
	}
	if compliance.HIPAA && minimum < 6*365*24*time.Hour {
		minimum = 6 * 365 * 24 * time.Hour
	}
	if compliance.ISO27001 && minimum < 3*365*24*time.Hour {
		minimum = 3 * 365 * 24 * time.Hour
	}
	return minimum
}
