package meter

import (
	"strings"
	"testing"
	"time"
)

func TestCmdTempParsesNumber(t *testing.T) {
	c := NewCmdTemp("echo 67.9", time.Millisecond)
	if got := c.TempC(); got != 67 {
		t.Errorf("TempC = %d, want 67", got)
	}
}

func TestCmdTempFailure(t *testing.T) {
	c := NewCmdTemp("false 2>/dev/null; echo oops", time.Millisecond)
	if got := c.TempC(); got != -1 {
		t.Errorf("TempC = %d, want -1 (нет числа)", got)
	}
	c2 := NewCmdTemp("exit 1", time.Millisecond)
	if got := c2.TempC(); got != -1 {
		t.Errorf("TempC = %d, want -1 (команда упала)", got)
	}
}

func TestCmdTempRejectsImplausible(t *testing.T) {
	c := NewCmdTemp("echo 999", time.Millisecond)
	if got := c.TempC(); got != -1 {
		t.Errorf("TempC = %d, want -1 (999° не бывает)", got)
	}
}

func TestCmdTempEmpty(t *testing.T) {
	if got := NewCmdTemp("", time.Second).TempC(); got != -1 {
		t.Errorf("TempC = %d, want -1", got)
	}
}

func TestCmdTempCache(t *testing.T) {
	// Первый вызов выполняет команду, второй — из кэша, даже если вывод
	// изменился: считаем, что за interval команда не перезапускается.
	c := NewCmdTemp("echo 40", time.Hour)
	if got := c.TempC(); got != 40 {
		t.Fatalf("TempC = %d, want 40", got)
	}
	if !strings.Contains(c.spec, "echo") {
		t.Error("spec потерян")
	}
	if got := c.TempC(); got != 40 {
		t.Errorf("TempC (кэш) = %d, want 40", got)
	}
}
