package ginkgo

#TestPlan: {
	Parallel: bool | *false
	Actions: [action=string]: #Run & {
		Name: action
	}
	Coverage: {
		MergeProfiles:     bool | *false
		MergedProfileName: string | *"cover.out"
		ExcludePatterns?: [...string]
	}
}
