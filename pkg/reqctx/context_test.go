package reqctx

import (
	"context"
	"testing"
	"time"

	"github.com/Educentr/go-onlineconf/pkg/onlineconf"
	"github.com/Educentr/go-onlineconf/pkg/onlineconf_dev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOnlineconf(t *testing.T) (context.Context, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	ctx, err := onlineconf.Initialize(context.Background(), onlineconf.WithConfigDir(tmpDir))
	require.NoError(t, err)

	onlineconf_dev.GenerateCDB(tmpDir, "TREE", map[string]interface{}{"test": "value"})

	err = onlineconf.StartWatcher(ctx)
	require.NoError(t, err)

	return ctx, func() { _ = onlineconf.StopWatcher(ctx) }
}

func TestCreateContextWithTimeout_ZeroTimeout(t *testing.T) {
	configCtx, cleanup := setupOnlineconf(t)
	defer cleanup()

	mainCtx := context.Background()

	ctx, cancel, err := CreateContextWithTimeout(mainCtx, configCtx, 0)
	require.NoError(t, err)
	defer cancel()

	// Zero timeout should not set a deadline
	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline, "zero timeout should not set a deadline")

	// Cleanup should not panic
	cancel()
}

func TestCreateContextWithTimeout_PositiveTimeout(t *testing.T) {
	configCtx, cleanup := setupOnlineconf(t)
	defer cleanup()

	mainCtx := context.Background()
	timeout := 5 * time.Second

	before := time.Now()

	ctx, cancel, err := CreateContextWithTimeout(mainCtx, configCtx, timeout)
	require.NoError(t, err)
	defer cancel()

	deadline, hasDeadline := ctx.Deadline()
	require.True(t, hasDeadline, "positive timeout should set a deadline")

	// Deadline should be approximately now + timeout
	expectedDeadline := before.Add(timeout)
	assert.WithinDuration(t, expectedDeadline, deadline, 100*time.Millisecond)
}

func TestCreateContextWithTimeout_TimeoutExpires(t *testing.T) {
	configCtx, cleanup := setupOnlineconf(t)
	defer cleanup()

	mainCtx := context.Background()

	ctx, cancel, err := CreateContextWithTimeout(mainCtx, configCtx, 50*time.Millisecond)
	require.NoError(t, err)
	defer cancel()

	time.Sleep(100 * time.Millisecond)

	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestCreateContextWithTimeout_InvalidConfigCtx(t *testing.T) {
	// context.Background() has no onlineconf instance — onlineconf.Clone panics
	// with nil pointer dereference. Verify CreateContextWithTimeout propagates this panic.
	assert.Panics(t, func() {
		_, _, _ = CreateContextWithTimeout(context.Background(), context.Background(), time.Second)
	})
}

func TestCreateContextWithTimeout_CancelReleasesResources(t *testing.T) {
	configCtx, cleanup := setupOnlineconf(t)
	defer cleanup()

	mainCtx := context.Background()

	ctx, cancel, err := CreateContextWithTimeout(mainCtx, configCtx, 5*time.Second)
	require.NoError(t, err)

	// Context should be valid before cancel
	assert.NoError(t, ctx.Err())

	// Cancel should release the cloned onlineconf context
	cancel()

	// After cancel, context should be done
	assert.Error(t, ctx.Err())
}
