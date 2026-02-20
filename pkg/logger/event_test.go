package logger

import (
	"context"
	"errors"
	"testing"
)

func TestFieldConstructors(t *testing.T) {
	f := Str("key", "value")
	if f.Key != "key" || f.Value != "value" {
		t.Errorf("Str() = %v, want {Key:key Value:value}", f)
	}

	f = Int("count", 42)
	if f.Key != "count" || f.Value != 42 {
		t.Errorf("Int() = %v, want {Key:count Value:42}", f)
	}

	f = Int64("big", 123456789)
	if f.Key != "big" || f.Value != int64(123456789) {
		t.Errorf("Int64() = %v, want {Key:big Value:123456789}", f)
	}

	f = Any("obj", []string{"a", "b"})
	if f.Key != "obj" {
		t.Errorf("Any() key = %q, want %q", f.Key, "obj")
	}
}

func TestGetEventLogger_ReturnsNopWhenNotSet(t *testing.T) {
	// Reset global
	old := globalEventLogger
	globalEventLogger = nil
	defer func() { globalEventLogger = old }()

	l := GetEventLogger()
	if l == nil {
		t.Fatal("GetEventLogger() should return nop logger, not nil")
	}

	// Should not panic
	l.Error(context.Background(), "test", errors.New("err"))
	l.Warn(context.Background(), "test")
	l.Info(context.Background(), "test")
}

func TestSetEventLogger(t *testing.T) {
	old := globalEventLogger
	defer func() { globalEventLogger = old }()

	mock := &mockEventLogger{}
	SetEventLogger(mock)

	l := GetEventLogger()
	if l != mock {
		t.Error("GetEventLogger() should return the logger set by SetEventLogger()")
	}
}

type mockEventLogger struct {
	lastMsg string
}

func (m *mockEventLogger) Error(_ context.Context, msg string, _ error, _ ...Field) {
	m.lastMsg = msg
}
func (m *mockEventLogger) Warn(_ context.Context, msg string, _ ...Field) { m.lastMsg = msg }
func (m *mockEventLogger) Info(_ context.Context, msg string, _ ...Field) { m.lastMsg = msg }
