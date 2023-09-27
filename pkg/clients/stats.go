package clients

type ConnStatsQuerier interface {
	QueryConnStats() (ConnStats, error)
}
