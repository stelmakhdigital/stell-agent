// Package agent — runtime-слой агента: цикл LLM+tools, сессии, хуки.
//
// Роль в монорепо: `github.com/stelmakhdigital/stell-agent` стоит между `github.com/stelmakhdigital/stell-ai` и `stell/coding-agent`.
// Продуктовая оркестрация (Service, extensions, discovery) живёт в coding-agent;
// этот пакет даёт Loop / Agent, хранилище сессий и встроенные инструменты.
//
// Основные импорты:
//
//   - github.com/stelmakhdigital/stell-agent — Loop, Agent, события, ConvertToLlm
//   - github.com/stelmakhdigital/stell-agent/session — JSONL-сессии и дерево веток
//   - github.com/stelmakhdigital/stell-agent/tools — runtime встроенных инструментов
//   - github.com/stelmakhdigital/stell-agent/hooks — in-process шина хуков
//   - github.com/stelmakhdigital/stell-agent/harness — компактирование / helpers system prompt
//   - github.com/stelmakhdigital/stell-agent/proxy — StreamFn через HTTP SSE
package agent
