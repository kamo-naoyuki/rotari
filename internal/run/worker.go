package run

type WorkerCallbacks struct {
	WriteContext  func() error
	MarkRunning   func() error
	StartSampling func() func()
	Execute       func() int
	FinishContext func() error
	Finalize      func(exitCode int) error
	RemoveLock    func() error
}

func RunWorker(callbacks WorkerCallbacks) (int, error) {
	if err := callbacks.WriteContext(); err != nil {
		return 1, err
	}
	if err := callbacks.MarkRunning(); err != nil {
		return 1, err
	}
	stopSampling := callbacks.StartSampling()
	exitCode := callbacks.Execute()
	stopSampling()
	if err := callbacks.FinishContext(); err != nil {
		return 1, err
	}
	if err := callbacks.Finalize(exitCode); err != nil {
		return 1, err
	}
	if err := callbacks.RemoveLock(); err != nil {
		return exitCode, err
	}
	return exitCode, nil
}
