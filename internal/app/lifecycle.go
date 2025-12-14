package app

import "context"

type ShutdownObserver interface {
	OnShutdown(ctx context.Context)
}

func (app *App) RegisterShutdownObserver(observer ShutdownObserver) {
	if observer == nil {
		return
	}
	app.shutdownObservers = append(app.shutdownObservers, observer)
}

func (app *App) notifyShutdownObservers(ctx context.Context) {
	for _, observer := range app.shutdownObservers {
		if observer == nil {
			continue
		}
		observer.OnShutdown(ctx)
	}
}
