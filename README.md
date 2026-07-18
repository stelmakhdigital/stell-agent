# github.com/stelmakhdigital/stell-agent

Runtime-слой агента: цикл LLM+tools, сессии, инструменты и хуки.

Продуктовая оркестрация (`Service`, extensions, discovery) живёт в `github.com/stelmakhdigital/stell-coding`
и либо управляет `Agent` там, либо вызывает `Loop` напрямую.

## Возможности

- `Loop` / `Run` / `RunContinue` — низкоуровневый ход LLM + tools
- `Agent` — очереди steer/follow-up и подписка на события
- `ConvertToLlm` / `PrepareMessages` — контекст сессии → сообщения LLM
- JSONL session store с деревом веток и компактированием
- Builtin tools runtime (`read` / `bash` / …)
- In-process hook bus

## Карта директорий

| Путь | Назначение |
|------|------------|
| `loop.go`, `agent.go`, `streamfn.go` | цикл хода, публичный Agent API, StreamFn |
| `events.go`, `convert.go`, `toolexec.go` | события, конвертация, исполнение tools |
| `session/` | JSONL-сессии, дерево, branch, compact |
| `tools/` | runtime и builtin tools |
| `hooks/` | имена хуков и Bus |
| `harness/` | оценка контекста, compaction helpers |
| `proxy/` | HTTP SSE StreamProxy |

## Использование

```go
import (
	"github.com/stelmakhdigital/stell-agent"
	"github.com/stelmakhdigital/stell-agent/session"
	"github.com/stelmakhdigital/stell-agent/tools"
	"github.com/stelmakhdigital/stell-ai/provider"
)

reg := provider.BuildRegistry()
rt := tools.NewRuntime()
tools.RegisterBuiltins(rt)
sess, _ := session.Open(...) // или session.NewManager

loop := &agent.Loop{Registry: reg, Tools: rt, Sessions: sess}
// ch, err := loop.Run(ctx, userMsg)
_ = loop
```
