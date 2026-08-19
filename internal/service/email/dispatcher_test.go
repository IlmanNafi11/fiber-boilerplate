package email

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	emaildomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeOutboxRepo is an in-memory stand-in for the outbox repository. It records
// mark calls so tests can assert on dispatch outcomes without a database.
type fakeOutboxRepo struct {
	mu       sync.Mutex
	pending  []*emaildomain.OutboxEvent
	sent     []string
	retries  []retryCall
	claimErr error
}

type retryCall struct {
	id            string
	nextAttemptAt time.Time
	lastError     string
	dead          bool
}

func (f *fakeOutboxRepo) ClaimBatch(_ context.Context, workerID string, limit int, _ time.Duration, _ time.Time) ([]*emaildomain.OutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	n := min(limit, len(f.pending))
	claimed := make([]*emaildomain.OutboxEvent, 0, n)
	for _, e := range f.pending[:n] {
		e.LockedBy = &workerID
		claimed = append(claimed, e)
	}
	f.pending = f.pending[n:]
	return claimed, nil
}

func (f *fakeOutboxRepo) MarkSent(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeOutboxRepo) MarkRetry(_ context.Context, id string, nextAttemptAt time.Time, lastError string, dead bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries = append(f.retries, retryCall{id, nextAttemptAt, lastError, dead})
	return nil
}

func (f *fakeOutboxRepo) sentIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *fakeOutboxRepo) retryCalls() []retryCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]retryCall(nil), f.retries...)
}

// recordingSender records what it was asked to send and can fail on demand.
type recordingSender struct {
	mu            sync.Mutex
	verifications []struct{ To, Token string }
	resets        []struct{ To, Token string }
	changed       []string
	err           error
}

func (r *recordingSender) SendVerificationEmail(_ context.Context, to, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.verifications = append(r.verifications, struct{ To, Token string }{to, token})
	return nil
}

func (r *recordingSender) SendPasswordResetEmail(_ context.Context, to, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.resets = append(r.resets, struct{ To, Token string }{to, token})
	return nil
}

func (r *recordingSender) SendPasswordChangedNotification(_ context.Context, to string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.changed = append(r.changed, to)
	return nil
}

func tokenEvent(t *testing.T, id, eventType, recipient, token string) *emaildomain.OutboxEvent {
	t.Helper()
	e, err := emaildomain.NewTokenEvent(eventType, recipient, token)
	require.NoError(t, err)
	e.ID = id
	e.Status = emaildomain.OutboxStatusPending
	return e
}

func testDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{
		Interval:    10 * time.Millisecond,
		BatchSize:   10,
		Lease:       time.Minute,
		SendTimeout: time.Second,
		MaxAttempts: 3,
		BaseBackoff: time.Second,
		MaxBackoff:  time.Hour,
	}
}

func TestDispatcher_DispatchesEachEventTypeToSender(t *testing.T) {
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{
		tokenEvent(t, "1", emaildomain.EventTypeVerification, "verify@example.com", "vtok"),
		tokenEvent(t, "2", emaildomain.EventTypePasswordReset, "reset@example.com", "rtok"),
		{ID: "3", EventType: emaildomain.EventTypePasswordChanged, Recipient: "changed@example.com", Payload: []byte("{}"), Status: emaildomain.OutboxStatusPending},
	}}
	sender := &recordingSender{}
	d := NewDispatcher(repo, sender, testDispatcherConfig(), zap.NewNop())

	n, err := d.dispatchOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	assert.Equal(t, []string{"1", "2", "3"}, repo.sentIDs())
	assert.Empty(t, repo.retryCalls())
	require.Len(t, sender.verifications, 1)
	assert.Equal(t, "verify@example.com", sender.verifications[0].To)
	assert.Equal(t, "vtok", sender.verifications[0].Token)
	require.Len(t, sender.resets, 1)
	assert.Equal(t, "rtok", sender.resets[0].Token)
	require.Len(t, sender.changed, 1)
	assert.Equal(t, "changed@example.com", sender.changed[0])
}

func TestDispatcher_TransientFailureReschedulesWithBackoff(t *testing.T) {
	evt := tokenEvent(t, "1", emaildomain.EventTypeVerification, "a@example.com", "tok")
	evt.Attempts = 1
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{evt}}
	sender := &recordingSender{err: errors.New("smtp down")}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	d := NewDispatcher(repo, sender, testDispatcherConfig(), zap.NewNop())
	d.now = func() time.Time { return now }

	_, err := d.dispatchOnce(context.Background())
	require.NoError(t, err)

	require.Len(t, repo.retryCalls(), 1)
	call := repo.retryCalls()[0]
	assert.Equal(t, "1", call.id)
	assert.False(t, call.dead)
	// attempts=1 → backoff = base * 2^1 = 2s
	assert.Equal(t, now.Add(2*time.Second), call.nextAttemptAt)
	assert.Empty(t, repo.sentIDs())
}

func TestDispatcher_ExhaustedAttemptsDeadLetters(t *testing.T) {
	evt := tokenEvent(t, "1", emaildomain.EventTypeVerification, "a@example.com", "tok")
	evt.Attempts = 2 // this attempt makes 3 == MaxAttempts
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{evt}}
	sender := &recordingSender{err: errors.New("smtp down")}

	d := NewDispatcher(repo, sender, testDispatcherConfig(), zap.NewNop())

	_, err := d.dispatchOnce(context.Background())
	require.NoError(t, err)

	require.Len(t, repo.retryCalls(), 1)
	assert.True(t, repo.retryCalls()[0].dead)
}

func TestDispatcher_UnknownEventTypeIsDeadLettered(t *testing.T) {
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{
		{ID: "1", EventType: "bogus", Recipient: "a@example.com", Payload: []byte("{}"), Status: emaildomain.OutboxStatusPending},
	}}
	sender := &recordingSender{}

	d := NewDispatcher(repo, sender, testDispatcherConfig(), zap.NewNop())
	_, err := d.dispatchOnce(context.Background())
	require.NoError(t, err)

	require.Len(t, repo.retryCalls(), 1)
	assert.True(t, repo.retryCalls()[0].dead, "unknown event type is a permanent failure")
	assert.Empty(t, repo.sentIDs())
}

func TestDispatcher_BackoffIsCapped(t *testing.T) {
	evt := tokenEvent(t, "1", emaildomain.EventTypeVerification, "a@example.com", "tok")
	evt.Attempts = 1
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{evt}}
	sender := &recordingSender{err: errors.New("smtp down")}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cfg := testDispatcherConfig()
	cfg.MaxBackoff = 1500 * time.Millisecond // below base*2^1 = 2s
	d := NewDispatcher(repo, sender, cfg, zap.NewNop())
	d.now = func() time.Time { return now }

	_, err := d.dispatchOnce(context.Background())
	require.NoError(t, err)

	require.Len(t, repo.retryCalls(), 1)
	assert.Equal(t, now.Add(1500*time.Millisecond), repo.retryCalls()[0].nextAttemptAt)
}

func TestDispatcher_RunStopsOnContextCancel(t *testing.T) {
	repo := &fakeOutboxRepo{pending: []*emaildomain.OutboxEvent{
		tokenEvent(t, "1", emaildomain.EventTypeVerification, "a@example.com", "tok"),
	}}
	sender := &recordingSender{}
	d := NewDispatcher(repo, sender, testDispatcherConfig(), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()

	// Wait for the first event to be delivered, then cancel.
	require.Eventually(t, func() bool {
		return len(repo.sentIDs()) == 1
	}, time.Second, 5*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestDispatcher_ImplementsInterface(t *testing.T) {
	var _ OutboxRepo = (*fakeOutboxRepo)(nil)
}

func TestNewFromConfig_MapsEmailConfig(t *testing.T) {
	cfg := config.EmailConfig{
		OutboxDispatchInterval: 3 * time.Second,
		OutboxBatchSize:        7,
		OutboxLease:            4 * time.Minute,
		OutboxSendTimeout:      9 * time.Second,
		OutboxMaxAttempts:      6,
		OutboxBaseBackoff:      11 * time.Second,
		OutboxMaxBackoff:       90 * time.Minute,
	}
	d := NewFromConfig(&fakeOutboxRepo{}, &recordingSender{}, cfg, zap.NewNop())
	require.NotNil(t, d)
	assert.Equal(t, DispatcherConfig{
		Interval:    3 * time.Second,
		BatchSize:   7,
		Lease:       4 * time.Minute,
		SendTimeout: 9 * time.Second,
		MaxAttempts: 6,
		BaseBackoff: 11 * time.Second,
		MaxBackoff:  90 * time.Minute,
	}, d.cfg)
}
