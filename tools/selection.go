package tools

// ToolSelection отражает CLI-флаги инструментов.
type ToolSelection struct {
	// Tools, если не пуст, — явный allowlist (заменяет значения по умолчанию).
	Tools []string
	// Exclude удаляет имена из активного набора.
	Exclude []string
	// NoTools не экспонирует инструменты LLM (встроенные могут оставаться зарегистрированными).
	NoTools bool
	// NoBuiltinTools пропускает регистрацию встроенных инструментов.
	NoBuiltinTools bool
	// IncludeCoding регистрирует и включает grep/find/ls в дополнение к core,
	// когда Tools пуст. Игнорируется, если Tools задан.
	IncludeCoding bool
}

// ResolveActiveTools возвращает список активных имён для SetActiveTools.
// Второй результат (restrict) всегда true: вызывающий применяет active к Runtime
// (пустой active при NoTools — не экспонировать инструменты LLM).
func ResolveActiveTools(sel ToolSelection, registered []string) (active []string, restrict bool) {
	if sel.NoTools {
		return []string{}, true
	}
	reg := map[string]bool{}
	for _, n := range registered {
		reg[n] = true
	}
	var base []string
	if len(sel.Tools) > 0 {
		for _, n := range sel.Tools {
			if reg[n] {
				base = append(base, n)
			}
		}
	} else {
		for _, n := range CoreTools {
			if reg[n] {
				base = append(base, n)
			}
		}
		if sel.IncludeCoding {
			for _, n := range CodingTools {
				if reg[n] {
					base = append(base, n)
				}
			}
		}
	}
	if len(sel.Exclude) == 0 {
		return base, true
	}
	ex := map[string]bool{}
	for _, n := range sel.Exclude {
		ex[n] = true
	}
	out := make([]string, 0, len(base))
	for _, n := range base {
		if !ex[n] {
			out = append(out, n)
		}
	}
	return out, true
}
