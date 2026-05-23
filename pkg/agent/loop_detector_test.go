package agent

import (
	"testing"
)

func TestLoopDetector_IdenticalToolCalls(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(3)

	params := map[string]string{"path": "/tmp/test.go"}

	// Первые два вызова — нормально
	isLoop, msg := d.RecordToolCall("read", params, true)
	if isLoop {
		t.Errorf("expected no loop on call 1, got: %s", msg)
	}

	isLoop, msg = d.RecordToolCall("read", params, true)
	if isLoop {
		t.Errorf("expected no loop on call 2, got: %s", msg)
	}

	// Третий вызов с теми же параметрами — зацикливание
	isLoop, msg = d.RecordToolCall("read", params, true)
	if !isLoop {
		t.Error("expected loop on call 3")
	}
	if msg == "" {
		t.Error("expected non-empty loop message")
	}
	t.Logf("Loop message: %s", msg)
}

func TestLoopDetector_DifferentToolCalls(t *testing.T) {
	d := NewLoopDetector()

	// Вызовы разных инструментов с разными параметрами — не зацикливание
	for i := 0; i < 3; i++ {
		// Каждый вызов уникальный (разные пути/паттерны)
		isLoop, _ := d.RecordToolCall("read", map[string]string{"path": "a" + string(rune('0'+i)) + ".go"}, true)
		if isLoop {
			t.Error("should not detect loop for different calls")
		}
		isLoop, _ = d.RecordToolCall("grep", map[string]string{"pattern": "test" + string(rune('0'+i))}, true)
		if isLoop {
			t.Error("should not detect loop for different calls")
		}
	}
}

func TestLoopDetector_PingPong(t *testing.T) {
	d := NewLoopDetector()

	paramsA := map[string]string{"path": "main.go"}
	paramsB := map[string]string{"pattern": "TODO"}

	// A(p1) → B(p2) → A(p1) → B(p2) → A(p1) → B(p2) — пинг-понг с одинаковыми параметрами
	calls := []struct {
		tool   string
		params map[string]string
	}{
		{"read", paramsA},
		{"grep", paramsB},
		{"read", paramsA},
		{"grep", paramsB},
		{"read", paramsA},
		{"grep", paramsB},
	}

	loopDetected := false
	for _, call := range calls {
		isLoop, msg := d.RecordToolCall(call.tool, call.params, true)
		if isLoop {
			loopDetected = true
			t.Logf("Ping-pong detected: %s", msg)
			break
		}
	}

	if !loopDetected {
		t.Error("expected ping-pong loop detection")
	}
}

func TestLoopDetector_PingPongDifferentParams(t *testing.T) {
	d := NewLoopDetector()

	// read(a.go) → grep(TODO) → read(b.go) → grep(FIXME) → read(c.go) → grep(BUG)
	// Разные параметры — это НЕ пинг-понг, модель работает с разными файлами
	calls := []struct {
		tool   string
		params map[string]string
	}{
		{"read", map[string]string{"path": "a.go"}},
		{"grep", map[string]string{"pattern": "TODO"}},
		{"read", map[string]string{"path": "b.go"}},
		{"grep", map[string]string{"pattern": "FIXME"}},
		{"read", map[string]string{"path": "c.go"}},
		{"grep", map[string]string{"pattern": "BUG"}},
	}

	for _, call := range calls {
		isLoop, msg := d.RecordToolCall(call.tool, call.params, true)
		if isLoop {
			t.Errorf("should not detect ping-pong for different params: %s", msg)
		}
	}
}

func TestLoopDetector_RepeatedTextResponse(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(3)

	text := "I cannot help with this request."

	// Два раза — не зацикливание
	isLoop, _ := d.RecordTextResponse(text)
	if isLoop {
		t.Error("should not detect loop on 2 identical texts")
	}

	isLoop, _ = d.RecordTextResponse(text)
	if isLoop {
		t.Error("should not detect loop on 2 identical texts")
	}

	// Третий раз — зацикливание
	isLoop, msg := d.RecordTextResponse(text)
	if !isLoop {
		t.Error("expected loop on 3 identical texts")
	}
	t.Logf("Text loop message: %s", msg)
}

func TestLoopDetector_DifferentTextResponses(t *testing.T) {
	d := NewLoopDetector()

	// Разные тексты с разным набором слов — не зацикливание
	texts := []string{
		"Сначала нужно прочитать файл конфигурации и понять структуру проекта.",
		"Теперь реализуем функцию парсинга аргументов командной строки.",
		"Добавим обработку ошибок для некорректных входных данных.",
		"Напишем юнит-тесты для всех публичных методов нового модуля.",
		"Сделаем рефакторинг: вынесем общую логику в отдельный пакет.",
		"Обновим документацию и добавим примеры использования.",
		"Проверим совместимость с предыдущей версией API.",
		"Оптимизируем запросы к базе данных для ускорения работы.",
		"Добавим кэширование результатов вычислений в памяти.",
		"Настроим CI/CD пайплайн для автоматического тестирования.",
	}
	for i, text := range texts {
		isLoop, msg := d.RecordTextResponse(text)
		if isLoop {
			t.Errorf("should not detect loop for different texts at %d: %s", i, msg)
		}
	}
}

func TestLoopDetector_ReadManyFiles(t *testing.T) {
	d := NewLoopDetector()

	// Модель читает 20 разных файлов подряд — это нормально
	for i := 0; i < 20; i++ {
		isLoop, _ := d.RecordToolCall("read", map[string]string{
			"path": "src/module" + string(rune('A'+i)) + "/main.go",
		}, true)
		if isLoop {
			t.Error("should not detect loop when reading many different files")
		}
	}
}

func TestLoopDetector_SameToolSameParams(t *testing.T) {
	d := NewLoopDetector()
	d.SetToolRepeatThreshold(4)
	d.SetRepeatThreshold(10) // Высокий порог для идентичных подряд вызовов

	params := map[string]string{"command": "go test"}

	// Один и тот же инструмент с одинаковыми параметрами — зацикливание (эвристика 2)
	for i := 0; i < 5; i++ {
		isLoop, msg := d.RecordToolCall("bash", params, true)
		if isLoop {
			t.Logf("Detected at call %d: %s", i+1, msg)
			if i < 3 {
				t.Errorf("detected too early at call %d", i+1)
			}
			return
		}
	}
	t.Error("expected loop detection for repeated tool calls")
}

func TestLoopDetector_WindowSize(t *testing.T) {
	d := NewLoopDetector()
	d.SetWindowSize(5)
	d.SetRepeatThreshold(3)

	// Заполняем окно разными вызовами
	for i := 0; i < 5; i++ {
		d.RecordToolCall("bash", map[string]string{"command": string(rune('0' + i))}, true)
	}

	// Теперь 3 одинаковых вызова — должно детектироваться
	for i := 0; i < 3; i++ {
		isLoop, msg := d.RecordToolCall("read", map[string]string{"path": "test.go"}, true)
		if isLoop {
			t.Logf("Detected after window fill: %s", msg)
			return
		}
	}
	t.Error("expected loop detection after window slide")
}

func TestLoopDetector_Reset(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(2)

	params := map[string]string{"path": "test.go"}

	// Два вызова — зацикливание
	d.RecordToolCall("read", params, true)
	isLoop, _ := d.RecordToolCall("read", params, true)
	if !isLoop {
		t.Error("expected loop")
	}

	// Сброс
	d.Reset()

	// После сброса — не должно быть зацикливания
	isLoop, _ = d.RecordToolCall("read", params, true)
	if isLoop {
		t.Error("should not detect loop after reset")
	}
}

func TestLoopDetector_Stats(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(100) // Высокий порог

	d.RecordToolCall("read", map[string]string{"path": "a.go"}, true)
	d.RecordToolCall("read", map[string]string{"path": "b.go"}, true)
	d.RecordToolCall("bash", map[string]string{"command": "ls"}, true)

	total, topPattern := d.Stats()
	if total != 3 {
		t.Errorf("expected 3 snapshots, got %d", total)
	}
	if topPattern == "" {
		t.Error("expected non-empty top pattern")
	}
	t.Logf("Stats: total=%d, topPattern=%s", total, topPattern)
}

func TestLoopDetector_FailedToolCalls(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(3)

	params := map[string]string{"path": "/nonexistent.go"}

	// Три вызова с ошибкой — тоже зацикливание (модель пытается снова и снова)
	for i := 0; i < 3; i++ {
		isLoop, msg := d.RecordToolCall("read", params, false)
		if isLoop {
			if i < 2 {
				t.Errorf("detected too early at call %d", i+1)
			}
			t.Logf("Failed tool loop detected: %s", msg)
			return
		}
	}
	t.Error("expected loop detection for repeated failed calls")
}

func TestLoopDetector_MixedToolAndText(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(3)

	// Чередование tool и text — не должно быть ложного срабатывания
	for i := 0; i < 10; i++ {
		d.RecordToolCall("read", map[string]string{"path": "file.go"}, true)
		d.RecordTextResponse("Here is the file content")
	}

	// Окно 20, но разные типы (tool vs text) — зацикливание по one-tool-only
	total, _ := d.Stats()
	t.Logf("After mixed calls: %d snapshots", total)
}

func TestHashToolCall_Deterministic(t *testing.T) {
	params := map[string]string{"path": "test.go", "content": "hello"}

	h1 := hashToolCall("write", params)
	h2 := hashToolCall("write", params)

	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
}

func TestHashToolCall_DifferentParams(t *testing.T) {
	h1 := hashToolCall("read", map[string]string{"path": "a.go"})
	h2 := hashToolCall("read", map[string]string{"path": "b.go"})

	if h1 == h2 {
		t.Error("different params should produce different hashes")
	}
}

func TestHashToolCall_DifferentTools(t *testing.T) {
	params := map[string]string{"path": "test.go"}

	h1 := hashToolCall("read", params)
	h2 := hashToolCall("write", params)

	if h1 == h2 {
		t.Error("different tools should produce different hashes")
	}
}

func TestLoopDetector_TextSimilarityLoop(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(100) // Высокий порог чтобы не срабатывала эвристика 3
	d.SetTextSimilarityWindow(4)
	d.SetTextSimilarityThreshold(0.65)

	// Симулируем «мыслительный цикл» — модель перефразирует одну и ту же мысль
	texts := []string{
		"MeshCfg имеет приватные поля. Нужно использовать MeshCfg::from_env(). Но для тестов это неудобно. Лучше создам обёртку, которая создаёт NeuroMesh с конфигурацией по умолчанию.",
		"MeshCfg имеет приватные поля. Нужно использовать MeshCfg::from_env(). Но для тестов это неудобно. Лучше создам обёртку, которая создаёт NeuroMesh с конфигурацией по умолчанию",
		"MeshCfg имеет приватные поля Нужно использовать MeshCfg from_env Но для тестов это неудобно Лучше создам обёртку которая создаёт NeuroMesh с конфигурацией по умолчанию",
		"MeshCfg имеет приватные поля Нужно использовать MeshCfg from_env Но для тестов неудобно Лучше создам обёртку которая создаёт NeuroMesh с конфигурацией по умолчанию",
	}

	loopDetected := false
	for i, text := range texts {
		isLoop, msg := d.RecordTextResponse(text)
		if isLoop {
			loopDetected = true
			t.Logf("Detected at text %d: %s", i+1, msg)
			break
		}
	}

	if !loopDetected {
		t.Error("expected text similarity loop detection for rephrased thoughts")
	}
}

func TestLoopDetector_TextSimilarityDifferentThoughts(t *testing.T) {
	d := NewLoopDetector()
	d.SetTextSimilarityWindow(4)
	d.SetTextSimilarityThreshold(0.65)

	// Разные мысли — не должно быть зацикливания
	texts := []string{
		"Сначала нужно прочитать файл конфигурации и понять структуру проекта.",
		"Теперь реализуем функцию парсинга аргументов командной строки.",
		"Добавим обработку ошибок для некорректных входных данных.",
		"Напишем юнит-тесты для всех публичных методов нового модуля.",
	}

	for i, text := range texts {
		isLoop, msg := d.RecordTextResponse(text)
		if isLoop {
			t.Errorf("should not detect loop for different thoughts at %d: %s", i+1, msg)
		}
	}
}

func TestExtractWords(t *testing.T) {
	// Проверяем что стоп-слова фильтруются
	words := extractWords("The quick brown fox jumps over the lazy dog")
	if _, ok := words["the"]; ok {
		t.Error("stopword 'the' should be filtered")
	}
	if _, ok := words["quick"]; !ok {
		t.Error("meaningful word 'quick' should be kept")
	}
	if _, ok := words["fox"]; !ok {
		t.Error("meaningful word 'fox' should be kept")
	}
}

func TestExtractWordsRussian(t *testing.T) {
	// Проверяем русские стоп-слова
	words := extractWords("Нужно использовать MeshCfg для создания конфигурации")
	if _, ok := words["нужно"]; !ok {
		t.Error("meaningful word 'нужно' should be kept")
	}
	if _, ok := words["meshcfg"]; !ok {
		t.Error("meaningful word 'meshcfg' should be kept")
	}
	if _, ok := words["для"]; ok {
		t.Error("stopword 'для' should be filtered")
	}
}

func TestLoopDetector_ThinkingLoop(t *testing.T) {
	d := NewLoopDetector()
	d.SetRepeatThreshold(100) // Высокий порог чтобы не срабатывала эвристика 3
	d.thinkingSimilarityWindow = 3
	d.thinkingSimilarityThreshold = 0.65

	// Симулируем «мыслительный цикл» — модель повторяет одни и те же размышления
	// (характерно для GLM-5.1 через z.ai)
	thoughts := []string{
		"Мне нужно прочитать файл main.go чтобы понять структуру проекта. Давайте откроем его.",
		"Мне нужно прочитать файл main.go чтобы понять структуру проекта. Давайте откроем его",
		"Мне нужно прочитать файл main.go чтобы понять структуру проекта. Давайте его откроем",
	}

	loopDetected := false
	for i, thought := range thoughts {
		isLoop, msg := d.RecordThinking(thought)
		if isLoop {
			loopDetected = true
			t.Logf("Thinking loop detected at thought %d: %s", i+1, msg)
			break
		}
	}

	if !loopDetected {
		t.Error("expected thinking loop detection for repetitive thinking blocks")
	}
}

func TestLoopDetector_ThinkingLoopDifferentThoughts(t *testing.T) {
	d := NewLoopDetector()
	d.thinkingSimilarityWindow = 3
	d.thinkingSimilarityThreshold = 0.65

	// Разные мысли — не должно быть зацикливания
	thoughts := []string{
		"Сначала нужно прочитать файл конфигурации и понять структуру проекта.",
		"Теперь реализуем функцию парсинга аргументов командной строки.",
		"Добавим обработку ошибок для некорректных входных данных.",
	}

	for i, thought := range thoughts {
		isLoop, msg := d.RecordThinking(thought)
		if isLoop {
			t.Errorf("should not detect thinking loop for different thoughts at %d: %s", i+1, msg)
		}
	}
}

func TestLoopDetector_ThinkingLoopReset(t *testing.T) {
	d := NewLoopDetector()
	d.thinkingSimilarityWindow = 3
	d.thinkingSimilarityThreshold = 0.65

	thought := "Мне нужно прочитать файл main.go чтобы понять структуру проекта."

	// Два thinking-блока — ещё не зацикливание (нужно 3)
	d.RecordThinking(thought)
	d.RecordThinking(thought)

	// Сброс
	d.Reset()

	// После сброса — не должно быть зацикливания
	isLoop, _ := d.RecordThinking(thought)
	if isLoop {
		t.Error("should not detect loop after reset")
	}
}

func TestLoopDetector_ThinkingAndToolMixed(t *testing.T) {
	d := NewLoopDetector()
	d.thinkingSimilarityWindow = 3

	// Чередование thinking и tool calls — не должно быть ложного срабатывания
	for i := 0; i < 5; i++ {
		d.RecordThinking("Размышляю о структуре файла " + string(rune('A'+i)))
		d.RecordToolCall("read", map[string]string{"path": "file" + string(rune('A'+i)) + ".go"}, true)
	}

	// Не должно быть зацикливания — разные мысли и разные вызовы
	total, _ := d.Stats()
	t.Logf("After mixed thinking+tool calls: %d snapshots", total)
}

func TestJaccardSimilarity(t *testing.T) {
	// Идентичные множества
	a := map[string]struct{}{"hello": {}, "world": {}, "test": {}}
	b := map[string]struct{}{"hello": {}, "world": {}, "test": {}}
	if sim := jaccardSimilarity(a, b); sim != 1.0 {
		t.Errorf("identical sets should have similarity 1.0, got %f", sim)
	}

	// Полностью разные множества
	c := map[string]struct{}{"foo": {}, "bar": {}}
	if sim := jaccardSimilarity(a, c); sim != 0.0 {
		t.Errorf("disjoint sets should have similarity 0.0, got %f", sim)
	}

	// Частичное пересечение
	d := map[string]struct{}{"hello": {}, "other": {}, "test": {}}
	sim := jaccardSimilarity(a, d)
	if sim < 0.4 || sim > 0.7 {
		t.Errorf("partial overlap should be ~0.5, got %f", sim)
	}

	// Пустые множества
	empty := map[string]struct{}{}
	if sim := jaccardSimilarity(empty, empty); sim != 1.0 {
		t.Errorf("two empty sets should have similarity 1.0, got %f", sim)
	}
	if sim := jaccardSimilarity(a, empty); sim != 0.0 {
		t.Errorf("non-empty vs empty should have similarity 0.0, got %f", sim)
	}
}
