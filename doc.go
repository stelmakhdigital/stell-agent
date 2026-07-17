// Package agent — runtime-слой агента: цикл LLM+tools, сессии, хуки.
//
// Роль в монорепо: `stell/agent` стоит между `github.com/stelmakhdigital/ai` и `stell/coding-agent`.
// Продуктовая оркестрация (Service, extensions, discovery) живёт в coding-agent;
// этот пакет даёт Loop / Agent, хранилище сессий и встроенные инструменты.
//
// Основные импорты:
//
//   - stell/agent — Loop, Agent, события, ConvertToLlm
//   - stell/agent/session — JSONL-сессии и дерево веток
//   - stell/agent/tools — runtime встроенных инструментов
//   - stell/agent/hooks — in-process шина хуков
//   - stell/agent/harness — компактирование / helpers system prompt
//   - stell/agent/proxy — StreamFn через HTTP SSE
package agent
