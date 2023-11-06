package clients

// Experimental on darwin systems
type ConnStatsQuerier interface {
	QueryConnStats() (ConnStats, error)
}
