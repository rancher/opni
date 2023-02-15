package v1

import (
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
)

func (e *AlertEndpoint) RedactSecrets() {
	if slack := e.GetSlack(); slack != nil {
		slack.WebhookUrl = storagev1.Redacted
	}
	if email := e.GetEmail(); email != nil {
		redacted := storagev1.Redacted
		email.SmtpAuthPassword = &redacted
	}
	if pg := e.GetPagerDuty(); pg != nil {
		pg.IntegrationKey = storagev1.Redacted
	}
}

func (a *AlertCondition) RedactSecrets() {}

func (e *AlertEndpoint) UnredactSecrets(unredacted *AlertEndpoint) {
	if !e.HasSameImplementation(unredacted) {
		return
	}
	if e.GetSlack() != nil && e.GetSlack().WebhookUrl == storagev1.Redacted {
		e.GetSlack().WebhookUrl = unredacted.GetSlack().WebhookUrl
	}
	if e.GetEmail() != nil && *e.GetEmail().SmtpAuthPassword == storagev1.Redacted {
		e.GetEmail().SmtpAuthPassword = unredacted.GetEmail().SmtpAuthPassword
	}
	if e.GetPagerDuty() != nil && e.GetPagerDuty().IntegrationKey == storagev1.Redacted {
		e.GetPagerDuty().IntegrationKey = unredacted.GetPagerDuty().IntegrationKey
	}
}

func (e *AlertEndpoint) HasSameImplementation(other *AlertEndpoint) bool {
	if e.GetSlack() != nil {
		return other.GetSlack() != nil
	}
	if e.GetEmail() != nil {
		return other.GetEmail() != nil
	}
	if e.GetPagerDuty() != nil {
		return other.GetPagerDuty() != nil
	}
	return false
}
