package hooks

type LoadingCompletedHook interface {
	Invoke(numLoaded int)
}

type onLoadingCompletedHook func(int)

func (h onLoadingCompletedHook) Invoke(numLoaded int) {
	h(numLoaded)
}

func OnLoadingCompleted(fn func(int)) LoadingCompletedHook {
	return onLoadingCompletedHook(fn)
}
