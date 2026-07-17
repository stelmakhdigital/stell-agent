package tools

import "context"

type progressKey struct{}

// ProgressFunc принимает частичный вывод инструмента (streaming progress).
type ProgressFunc func(partial string)

// WithProgress сохраняет ProgressFunc в контексте для вызова инструмента.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, fn)
}

// ProgressFrom извлекает ProgressFunc из контекста (может быть nil).
func ProgressFrom(ctx context.Context) ProgressFunc {
	fn, _ := ctx.Value(progressKey{}).(ProgressFunc)
	return fn
}
