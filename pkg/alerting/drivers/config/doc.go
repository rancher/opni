// Embeds in `api_types.go“:
// - root level AlertManager configurations & configuration types
// - notifier configurations
//
// Repo : github.com/prometheus/alertmanager
// Path:
// - config/config.go
// - config/notifiers.go
// Changelist :
// 1) Secret/SecretURL fields are marshalled and unmarshalled as strings
// 2) We extend methods on each notifier struct in this package's receiver.go/message.go file.
package config
