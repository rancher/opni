package ginkgo

#TestPlan: {
	parallel: bool | *false
	actions: [action=string]: #Run & {
		name: action
	}
	coverage: {
		mergeReports: bool | *false
		mergedCoverProfile: string | *"cover.out"
	}
}
