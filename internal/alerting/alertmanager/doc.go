// Embeds AlertManager main function into our own runner.
// Repo : github.com/prometheus/alertmanager
// Path: cmd/alertmanager/main.go
//
// Changelist :
// 1) We give the main function the `args []string` input and treat it as os.Args
// 2) We embed opni flags that are use to configure the opni embedded server
// 3) The opni embedded server is a default hook for alertmanager to send notifications to.
package alertmanager_internal
