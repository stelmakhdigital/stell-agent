package agent

import (
	"context"
	"sync"

	"github.com/stelmakhdigital/ai"
	"stell/agent/tools"
)

// ToolExecutionMode — последовательное или параллельное выполнение tool calls в одном ходе assistant.
type ToolExecutionMode string

const (
	ToolExecutionSequential ToolExecutionMode = "sequential"
	ToolExecutionParallel   ToolExecutionMode = "parallel"
)

// ToolCallOutcome — один результат выполненного инструмента в порядке вызовов assistant.
type ToolCallOutcome struct {
	Call      ai.ToolCall
	Result    tools.Result
	Err       error
	Terminate bool // terminate — batch останавливается, когда все true
}

// ShouldTerminateToolBatch возвращает true, если batch не пуст и у всех outcome установлен Terminate.
func ShouldTerminateToolBatch(outcomes []ToolCallOutcome) bool {
	if len(outcomes) == 0 {
		return false
	}
	for _, oc := range outcomes {
		if !oc.Terminate && !oc.Result.Terminate {
			return false
		}
	}
	return true
}

// ExecuteToolCalls выполняет tool calls по порядку (sequential) или параллельно.
// Результаты всегда возвращаются в том же порядке, что и вызовы.
// onProgress может быть nil; при parallel progress best-effort для каждого вызова.
func ExecuteToolCalls(
	ctx context.Context,
	rt *tools.Runtime,
	calls []ai.ToolCall,
	mode ToolExecutionMode,
	onProgress func(call ai.ToolCall, partial string),
) []ToolCallOutcome {
	out := make([]ToolCallOutcome, len(calls))
	if len(calls) == 0 || rt == nil {
		return out
	}
	if mode != ToolExecutionParallel || len(calls) == 1 {
		for i, call := range calls {
			if ctx.Err() != nil {
				out[i] = ToolCallOutcome{Call: call, Err: ctx.Err()}
				continue
			}
			pctx := ctx
			if onProgress != nil {
				c := call
				pctx = tools.WithProgress(ctx, func(partial string) { onProgress(c, partial) })
			}
			res, err := rt.Execute(pctx, call)
			out[i] = ToolCallOutcome{Call: call, Result: res, Err: err}
		}
		return out
	}

	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call ai.ToolCall) {
			defer wg.Done()
			pctx := ctx
			if onProgress != nil {
				c := call
				pctx = tools.WithProgress(ctx, func(partial string) { onProgress(c, partial) })
			}
			res, err := rt.Execute(pctx, call)
			out[i] = ToolCallOutcome{Call: call, Result: res, Err: err}
		}(i, call)
	}
	wg.Wait()
	return out
}
