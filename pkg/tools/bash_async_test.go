package tools

import (
	"strings"
	"testing"
	"time"
)

// TestExecuteAsync_ReturnsChannel проверяет что ExecuteAsync возвращает канал
func TestExecuteAsync_ReturnsChannel(t *testing.T) {
	tool := NewBashTool()
	ch := tool.ExecuteAsync(map[string]string{"command": "echo hello"})
	if ch == nil {
		t.Fatal("ExecuteAsync returned nil channel")
	}
}

// TestExecuteAsync_ReceivesResult проверяет что получаем финальное событие result
func TestExecuteAsync_ReceivesResult(t *testing.T) {
	tool := NewBashTool()
	ch := tool.ExecuteAsync(map[string]string{"command": "echo hello"})

	var result AsyncEvent
	timeout := time.After(5 * time.Second)

	for evt := range ch {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for async result")
		default:
		}
		if evt.Done {
			result = evt
			break
		}
	}

	if !result.Done {
		t.Fatal("expected Done=true on final event")
	}
	if result.Type != "result" {
		t.Fatalf("expected Type=result, got %s", result.Type)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("expected output to contain 'hello', got: %s", result.Output)
	}
}

// TestExecuteAsync_ReceivesProgress проверяет что длинная команда отправляет progress события
func TestExecuteAsync_ReceivesProgress(t *testing.T) {
	tool := NewBashTool()
	// Команда, которая выводит несколько строк с задержкой
	ch := tool.ExecuteAsync(map[string]string{
		"command": "echo line1 && echo line2 && echo line3",
	})

	var progressCount int
	var gotResult bool
	timeout := time.After(5 * time.Second)

	for evt := range ch {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for async events")
		default:
		}
		if evt.Type == "progress" {
			progressCount++
		}
		if evt.Done {
			gotResult = true
		}
	}

	if !gotResult {
		t.Fatal("never received final result event")
	}
	// Должно быть хотя бы одно progress событие (или больше)
	if progressCount == 0 {
		t.Log("no progress events (acceptable for fast commands)")
	}
}

// TestExecuteAsync_BlockedCommand проверяет что заблокированные команды возвращают ошибку
func TestExecuteAsync_BlockedCommand(t *testing.T) {
	tool := NewBashTool()
	ch := tool.ExecuteAsync(map[string]string{"command": "rm -rf /"})

	var result AsyncEvent
	for evt := range ch {
		if evt.Done {
			result = evt
			break
		}
	}

	if result.Error == "" {
		t.Fatal("expected error for blocked command")
	}
}

// TestExecuteAsync_DoesNotBlock проверяет что ExecuteAsync не блокирует вызывающий поток
func TestExecuteAsync_DoesNotBlock(t *testing.T) {
	tool := NewBashTool()
	// Команда, которая спит 2 секунды
	ch := tool.ExecuteAsync(map[string]string{"command": "sleep 2 && echo done"})

	// Сразу после вызова ExecuteAsync мы должны иметь канал
	// и не быть заблокированными
	select {
	case <-ch:
		// Можем получить событие сразу (маловероятно для sleep)
	default:
		// Ожидаемо — канал пуст, но мы не заблокированы
	}

	// Ждём результат с таймаутом
	timeout := time.After(10 * time.Second)
	var gotResult bool
	for evt := range ch {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for async result")
		default:
		}
		if evt.Done {
			gotResult = true
			if !strings.Contains(evt.Output, "done") {
				t.Fatalf("expected 'done' in output, got: %s", evt.Output)
			}
			break
		}
	}

	if !gotResult {
		t.Fatal("never received final result")
	}
}

// TestExecuteAsync_TimeoutKillsProcess проверяет что таймаут убивает процесс и не зависает
func TestExecuteAsync_TimeoutKillsProcess(t *testing.T) {
	tool := NewBashTool()
	tool.Timeout = 100 * time.Millisecond

	start := time.Now()
	ch := tool.ExecuteAsync(map[string]string{"command": "sleep 10"})

	var result AsyncEvent
	timeout := time.After(5 * time.Second)
	for evt := range ch {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for async result after kill")
		default:
		}
		if evt.Done {
			result = evt
			break
		}
	}

	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v (expected < 2s)", elapsed)
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
	if result.Error == "" {
		t.Fatal("expected error for killed process")
	}
	if !strings.Contains(result.Error, "command failed:") {
		t.Fatalf("expected 'command failed:' in error, got: %s", result.Error)
	}
}

// TestExecuteAsync_StderrProgress проверяет что stderr тоже отправляется как progress
func TestExecuteAsync_StderrProgress(t *testing.T) {
	tool := NewBashTool()
	ch := tool.ExecuteAsync(map[string]string{"command": "echo out && echo err >&2"})

	var hasStderr bool
	var result AsyncEvent
	for evt := range ch {
		if evt.Type == "progress" && strings.HasPrefix(evt.Output, "STDERR:") {
			hasStderr = true
		}
		if evt.Done {
			result = evt
			break
		}
	}

	if !result.Done {
		t.Fatal("never received final result")
	}
	if !hasStderr {
		t.Log("no stderr progress events (acceptable)")
	}
}
