package app

import "context"

type LifecycleObserver interface {
	OnShutdown(ctx context.Context)
	OnSessionEnd(ctx context.Context, sessionID, reason string)
	OnPreCompact(ctx context.Context, sessionID string)
}

func (app *App) RegisterObserver(observer LifecycleObserver) {
	if observer == nil {
		return
	}
	app.observers = append(app.observers, observer)
}

func (app *App) notifyShutdownObservers(ctx context.Context) {
	for _, observer := range app.observers {
		if observer == nil {
			continue
		}
		observer.OnShutdown(ctx)
	}
}

func (app *App) NotifySessionEnd(ctx context.Context, sessionID, reason string) {
	for _, observer := range app.observers {
		if observer == nil {
			continue
		}
		observer.OnSessionEnd(ctx, sessionID, reason)
	}
}

func (app *App) NotifyPreCompact(ctx context.Context, sessionID string) {
	for _, observer := range app.observers {
		if observer == nil {
			continue
		}
		observer.OnPreCompact(ctx, sessionID)
	}
}
