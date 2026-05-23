package tools

import (
	"sync"
	"testing"
	"time"
)

func TestAskUserTool_AskChannel(t *testing.T) {
	tool := NewAskUserTool()

	// Устанавливаем AskChannel
	ch := &AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	tool.SetAskChannel(ch)

	var result ToolResult
	var wg sync.WaitGroup
	wg.Add(1)

	// Запускаем Execute в горутине — он отправит вопрос и заблокируется на Answer
	go func() {
		defer wg.Done()
		result = tool.Execute(map[string]string{"question": "What is your name?"})
	}()

	// Ждём вопрос из канала
	select {
	case question := <-ch.Question:
		if question != "What is your name?" {
			t.Errorf("Expected question 'What is your name?', got '%s'", question)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for question")
	}

	// Отправляем ответ
	ch.Answer <- "Alice"

	// Ждём завершения Execute
	wg.Wait()

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output != "Alice" {
		t.Errorf("Expected output 'Alice', got '%s'", result.Output)
	}
}

func TestAskUserTool_AskChannel_EmptyAnswer(t *testing.T) {
	tool := NewAskUserTool()

	ch := &AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	tool.SetAskChannel(ch)

	var result ToolResult
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		result = tool.Execute(map[string]string{"question": "Name?"})
	}()

	<-ch.Question
	ch.Answer <- "" // Пустой ответ

	wg.Wait()

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Пустой ответ должен вернуть no_answer (i18n-ключ)
	if result.Output == "" {
		t.Error("Expected non-empty output for empty answer (no_answer message)")
	}
}

func TestAskUserTool_AskChannel_NoChannel(t *testing.T) {
	tool := NewAskUserTool()
	// Без AskChannel и без NonInteractive — попытается читать stdin
	// Проверяем что SetAskChannel(nil) очищает канал
	tool.SetAskChannel(nil)

	// NonInteractive — чтобы не блокироваться на stdin
	tool.NonInteractive = true
	result := tool.Execute(map[string]string{"question": "Test?"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
}

func TestAskUserTool_ExecuteAsync_WithChannel(t *testing.T) {
	tool := NewAskUserTool()

	ch := &AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	tool.SetAskChannel(ch)

	// Запускаем ExecuteAsync
	asyncCh := tool.ExecuteAsync(map[string]string{"question": "Color?"})

	// Ждём вопрос
	select {
	case question := <-ch.Question:
		if question != "Color?" {
			t.Errorf("Expected 'Color?', got '%s'", question)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for question")
	}

	// Отправляем ответ
	ch.Answer <- "blue"

	// Читаем результат из async-канала
	var event AsyncEvent
	select {
	case event = <-asyncCh:
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for async result")
	}

	if !event.Done {
		t.Error("Expected Done=true")
	}
	if event.Error != "" {
		t.Errorf("Unexpected error: %s", event.Error)
	}
	if event.Output != "blue" {
		t.Errorf("Expected output 'blue', got '%s'", event.Output)
	}
}

func TestAskUserTool_ExecuteAsync_NonInteractive(t *testing.T) {
	tool := NewAskUserTool()
	tool.NonInteractive = true

	asyncCh := tool.ExecuteAsync(map[string]string{"question": "Test?"})

	var event AsyncEvent
	select {
	case event = <-asyncCh:
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for async result")
	}

	if !event.Done {
		t.Error("Expected Done=true")
	}
	if event.Error != "" {
		t.Errorf("Unexpected error: %s", event.Error)
	}
}

func TestAskUserTool_AskFunc(t *testing.T) {
	tool := NewAskUserTool()

	// Устанавливаем AskFunc — имитирует readline
	tool.SetAskFunc(func(question string) string {
		if question != "What is your name?" {
			t.Errorf("Expected question 'What is your name?', got '%s'", question)
		}
		return "Alice"
	})

	result := tool.Execute(map[string]string{"question": "What is your name?"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output != "Alice" {
		t.Errorf("Expected output 'Alice', got '%s'", result.Output)
	}
}

func TestAskUserTool_AskFunc_EmptyAnswer(t *testing.T) {
	tool := NewAskUserTool()

	tool.SetAskFunc(func(question string) string {
		return "" // Пустой ответ
	})

	result := tool.Execute(map[string]string{"question": "Name?"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Пустой ответ → no_answer
	if result.Output == "" {
		t.Error("Expected non-empty output for empty answer")
	}
}

func TestAskUserTool_AskFunc_PriorityOverFallback(t *testing.T) {
	tool := NewAskUserTool()
	// Без AskChannel и без AskFunc — fallback (no_answer)
	result := tool.Execute(map[string]string{"question": "Test?"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Должен вернуть no_answer
	if result.Output == "" {
		t.Error("Expected non-empty output for fallback")
	}
}

func TestAskUserTool_AskChannel_PriorityOverAskFunc(t *testing.T) {
	tool := NewAskUserTool()

	// Устанавливаем и AskChannel, и AskFunc — AskChannel должен иметь приоритет
	ch := &AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	tool.SetAskChannel(ch)
	tool.SetAskFunc(func(question string) string {
		return "from_askfunc" // Не должен вызываться
	})

	var result ToolResult
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		result = tool.Execute(map[string]string{"question": "Priority?"})
	}()

	<-ch.Question
	ch.Answer <- "from_channel"

	wg.Wait()

	if result.Output != "from_channel" {
		t.Errorf("Expected 'from_channel', got '%s'", result.Output)
	}
}

func TestAskUserTool_SetAskChannel_ConcurrentSafe(t *testing.T) {
	tool := NewAskUserTool()

	// Параллельная установка канала не должна вызывать панику
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := &AskChannel{
				Question: make(chan string, 1),
				Answer:   make(chan string, 1),
			}
			tool.SetAskChannel(ch)
		}()
	}
	wg.Wait()
}
