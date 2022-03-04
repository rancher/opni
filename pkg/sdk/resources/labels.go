package resources

func Labels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "opni-monitoring",
	}
}
