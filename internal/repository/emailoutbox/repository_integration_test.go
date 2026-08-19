//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	"github.com/ilmannafi/fiber-boilerplate/testhelpers"
)

type OutboxSuite struct {
	suite.Suite
	container *postgres.PostgresContainer
	pool      *pgxpool.Pool
	repo      *Repository
}

func (s *OutboxSuite) SetupSuite() {
	ctx := context.Background()
	container, pool := testhelpers.StartPostgres(ctx, s.T())
	s.container = container
	s.pool = pool

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(s.T(), err)
	testhelpers.RunMigrations(s.T(), dsn)

	s.repo = NewRepository(pool)
}

func (s *OutboxSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		if err := s.container.Terminate(context.Background()); err != nil {
			s.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (s *OutboxSuite) SetupTest() {
	testhelpers.TruncateAllTables(s.T(), s.pool)
}

// enqueue is a helper that enqueues one event inside its own committed tx.
func (s *OutboxSuite) enqueue(eventType, recipient string, payload []byte) *email.OutboxEvent {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	require.NoError(s.T(), err)

	evt := &email.OutboxEvent{
		EventType: eventType,
		Recipient: recipient,
		Payload:   payload,
	}
	require.NoError(s.T(), s.repo.EnqueueWithTx(ctx, tx, evt))
	require.NoError(s.T(), tx.Commit(ctx))
	return evt
}

func (s *OutboxSuite) TestEnqueueWithTx_PopulatesDefaults() {
	evt := s.enqueue("verification", "user@example.com", []byte(`{"token":"abc"}`))

	require.NotEmpty(s.T(), evt.ID)
	assert.Equal(s.T(), email.OutboxStatusPending, evt.Status)
	assert.Equal(s.T(), 0, evt.Attempts)
	assert.False(s.T(), evt.CreatedAt.IsZero())
}

func (s *OutboxSuite) TestEnqueueWithTx_RollbackDropsEvent() {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	require.NoError(s.T(), err)

	evt := &email.OutboxEvent{EventType: "verification", Recipient: "r@example.com", Payload: []byte(`{}`)}
	require.NoError(s.T(), s.repo.EnqueueWithTx(ctx, tx, evt))
	require.NoError(s.T(), tx.Rollback(ctx))

	var count int
	require.NoError(s.T(), s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM email_outbox").Scan(&count))
	assert.Equal(s.T(), 0, count)
}

func (s *OutboxSuite) TestClaimBatch_ReturnsDuePending() {
	s.enqueue("verification", "a@example.com", []byte(`{}`))
	s.enqueue("verification", "b@example.com", []byte(`{}`))

	claimed, err := s.repo.ClaimBatch(context.Background(), "worker-1", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	assert.Len(s.T(), claimed, 2)
	for _, c := range claimed {
		require.NotNil(s.T(), c.LockedBy)
		assert.Equal(s.T(), "worker-1", *c.LockedBy)
		require.NotNil(s.T(), c.LockedAt)
	}
}

func (s *OutboxSuite) TestClaimBatch_RespectsLimit() {
	for range 3 {
		s.enqueue("verification", "x@example.com", []byte(`{}`))
	}

	claimed, err := s.repo.ClaimBatch(context.Background(), "worker-1", 2, time.Minute, time.Now())
	require.NoError(s.T(), err)
	assert.Len(s.T(), claimed, 2)
}

func (s *OutboxSuite) TestClaimBatch_SkipsNotYetDue() {
	// next_attempt_at in the future via MarkRetry.
	evt := s.enqueue("verification", "future@example.com", []byte(`{}`))
	claimed, err := s.repo.ClaimBatch(context.Background(), "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	require.Len(s.T(), claimed, 1)
	require.NoError(s.T(), s.repo.MarkRetry(context.Background(), evt.ID, time.Now().Add(time.Hour), "boom", false))

	claimed, err = s.repo.ClaimBatch(context.Background(), "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	assert.Empty(s.T(), claimed)
}

func (s *OutboxSuite) TestClaimBatch_ConcurrentWorkersDoNotDoubleClaim() {
	const n = 20
	for range n {
		s.enqueue("verification", "c@example.com", []byte(`{}`))
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]int{}

	for w := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			claimed, err := s.repo.ClaimBatch(context.Background(), "worker", 10, time.Minute, time.Now())
			assert.NoError(s.T(), err)
			mu.Lock()
			for _, c := range claimed {
				seen[c.ID]++
			}
			mu.Unlock()
		}(w)
	}
	wg.Wait()

	assert.Len(s.T(), seen, n, "every claimed event must be unique across workers")
	for id, count := range seen {
		assert.Equal(s.T(), 1, count, "event %s claimed more than once", id)
	}
}

func (s *OutboxSuite) TestClaimBatch_ExpiredLeaseIsReclaimed() {
	ctx := context.Background()
	s.enqueue("verification", "lease@example.com", []byte(`{}`))

	// Worker 1 claims but never acks.
	first, err := s.repo.ClaimBatch(ctx, "worker-1", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	require.Len(s.T(), first, 1)

	// With a fresh lease, worker 2 sees nothing.
	none, err := s.repo.ClaimBatch(ctx, "worker-2", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	require.Empty(s.T(), none)

	// Simulate lease expiry: claim with now advanced past the lease window.
	future := time.Now().Add(2 * time.Minute)
	reclaimed, err := s.repo.ClaimBatch(ctx, "worker-2", 10, time.Minute, future)
	require.NoError(s.T(), err)
	require.Len(s.T(), reclaimed, 1)
	require.NotNil(s.T(), reclaimed[0].LockedBy)
	assert.Equal(s.T(), "worker-2", *reclaimed[0].LockedBy)
	assert.Equal(s.T(), first[0].ID, reclaimed[0].ID)
}

func (s *OutboxSuite) TestMarkSent_MakesEventUnclaimable() {
	ctx := context.Background()
	s.enqueue("verification", "sent@example.com", []byte(`{}`))

	claimed, err := s.repo.ClaimBatch(ctx, "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	require.Len(s.T(), claimed, 1)

	require.NoError(s.T(), s.repo.MarkSent(ctx, claimed[0].ID))

	var status string
	require.NoError(s.T(), s.pool.QueryRow(ctx, "SELECT status FROM email_outbox WHERE id = $1", claimed[0].ID).Scan(&status))
	assert.Equal(s.T(), email.OutboxStatusSent, status)

	// Even after lease expiry, a sent event is never reclaimed.
	future := time.Now().Add(2 * time.Minute)
	again, err := s.repo.ClaimBatch(ctx, "w2", 10, time.Minute, future)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), again)
}

// TestMarkSent_ClearsPayload guards that MarkSent wipes the payload so single-use
// tokens do not persist in plaintext after delivery. It enqueues a NON-empty
// token payload (an empty starting payload would make the assertion vacuous) and
// asserts the row's payload is {} once sent. Fails if the payload='{}'::jsonb
// clear in MarkSent is removed.
func (s *OutboxSuite) TestMarkSent_ClearsPayload() {
	ctx := context.Background()
	s.enqueue("verification", "token@example.com", []byte(`{"token":"super-secret-token"}`))

	claimed, err := s.repo.ClaimBatch(ctx, "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)
	require.Len(s.T(), claimed, 1)
	require.JSONEq(s.T(), `{"token":"super-secret-token"}`, string(claimed[0].Payload),
		"claimed event must still carry the token for delivery")

	require.NoError(s.T(), s.repo.MarkSent(ctx, claimed[0].ID))

	var payload []byte
	require.NoError(s.T(), s.pool.QueryRow(ctx,
		"SELECT payload FROM email_outbox WHERE id = $1", claimed[0].ID).Scan(&payload))
	assert.JSONEq(s.T(), `{}`, string(payload), "MarkSent must clear the payload plaintext token")
}

func (s *OutboxSuite) TestMarkRetry_ReschedulesAndRecordsError() {
	ctx := context.Background()
	evt := s.enqueue("verification", "retry@example.com", []byte(`{}`))

	_, err := s.repo.ClaimBatch(ctx, "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)

	next := time.Now().Add(30 * time.Second)
	require.NoError(s.T(), s.repo.MarkRetry(ctx, evt.ID, next, "smtp timeout", false))

	var status string
	var attempts int
	var lastErr *string
	var lockedBy *string
	require.NoError(s.T(), s.pool.QueryRow(ctx,
		"SELECT status, attempts, last_error, locked_by FROM email_outbox WHERE id = $1", evt.ID,
	).Scan(&status, &attempts, &lastErr, &lockedBy))

	assert.Equal(s.T(), email.OutboxStatusPending, status)
	assert.Equal(s.T(), 1, attempts)
	require.NotNil(s.T(), lastErr)
	assert.Equal(s.T(), "smtp timeout", *lastErr)
	assert.Nil(s.T(), lockedBy, "retry must release the lease")
}

func (s *OutboxSuite) TestMarkRetry_DeadLetterStops() {
	ctx := context.Background()
	evt := s.enqueue("verification", "dead@example.com", []byte(`{}`))

	_, err := s.repo.ClaimBatch(ctx, "w", 10, time.Minute, time.Now())
	require.NoError(s.T(), err)

	require.NoError(s.T(), s.repo.MarkRetry(ctx, evt.ID, time.Now(), "permanent", true))

	var status string
	require.NoError(s.T(), s.pool.QueryRow(ctx, "SELECT status FROM email_outbox WHERE id = $1", evt.ID).Scan(&status))
	assert.Equal(s.T(), email.OutboxStatusDead, status)

	// Dead events are never reclaimed.
	future := time.Now().Add(2 * time.Minute)
	claimed, err := s.repo.ClaimBatch(ctx, "w2", 10, time.Minute, future)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), claimed)
}

func TestOutboxSuite(t *testing.T) {
	suite.Run(t, new(OutboxSuite))
}
