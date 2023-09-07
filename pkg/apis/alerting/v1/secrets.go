package v1

import (
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
)

// TODO : new proto config logic will abstract away these method implementations

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
	if wh := e.GetWebhook(); wh != nil {
		wh.RedactSecrets()
	}
}

func (w *WebhookEndpoint) RedactSecrets() {
	w.Url = storagev1.Redacted
	if w.HttpConfig != nil {
		if w.HttpConfig.BasicAuth != nil {
			w.HttpConfig.BasicAuth.Password = storagev1.Redacted
		}
		if w.HttpConfig.Authorization != nil {
			w.HttpConfig.Authorization.Credentials = storagev1.Redacted
		}
		if w.HttpConfig.Oauth2 != nil {
			if w.HttpConfig.Oauth2.ClientSecret != "" {
				w.HttpConfig.Oauth2.ClientSecret = storagev1.Redacted
			}
		}
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
	if e.GetWebhook() != nil {
		e.GetWebhook().UnredactSecrets(unredacted.GetWebhook())
	}

}

func (w *WebhookEndpoint) UnredactSecrets(unredacted *WebhookEndpoint) {
	w.Url = unredacted.GetUrl()
	if w.HttpConfig != nil && unredacted != nil {
		if w.HttpConfig.BasicAuth != nil && unredacted.HttpConfig.BasicAuth != nil {
			w.HttpConfig.BasicAuth.Password = unredacted.HttpConfig.BasicAuth.Password
		}
		if w.HttpConfig.Authorization != nil && unredacted.HttpConfig.Authorization != nil {
			w.HttpConfig.Authorization.Credentials = unredacted.HttpConfig.Authorization.Credentials
		}
		if w.HttpConfig.Oauth2 != nil && unredacted.HttpConfig.Oauth2 != nil {
			if w.HttpConfig.Oauth2.ClientSecret != "" {
				w.HttpConfig.Oauth2.ClientSecret = unredacted.HttpConfig.Oauth2.ClientSecret
			}
		}
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
	if e.GetWebhook() != nil {
		return other.GetWebhook() != nil
	}
	return false
}
