package scheduler

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/fussraider/PopuGate/pkg/logger"
)

var log = logger.WithScope("scheduler")

const defaultTaskTimeout = 30 * time.Second

// Scheduler runs periodic tasks.
type Scheduler struct {
	cron   *cron.Cron
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Scheduler.
func New() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:   cron.New(cron.WithSeconds()),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Task is a periodic task definition.
type Task struct {
	Name     string
	Schedule string        // cron expression
	Timeout  time.Duration // per-task timeout (0 = defaultTaskTimeout)
	Fn       func(ctx context.Context) error
}

// Start begins all scheduled tasks.
func (s *Scheduler) Start(tasks []Task) {
	for _, t := range tasks {
		if t.Fn == nil {
			log.Warnf("skip scheduling %s: no function provided", t.Name)
			continue
		}
		task := t // capture
		_, err := s.cron.AddFunc(task.Schedule, func() {
			timeout := task.Timeout
			if timeout == 0 {
				timeout = defaultTaskTimeout
			}
			ctx, cancel := context.WithTimeout(s.ctx, timeout)
			defer cancel()

			if err := task.Fn(ctx); err != nil {
				log.Errorf("%s error: %v", task.Name, err)
			}
		})
		if err != nil {
			log.Errorf("failed to schedule %s: %v", task.Name, err)
		} else {
			log.Infof("scheduled: %s (%s)", task.Name, task.Schedule)
		}
	}
	s.cron.Start()
}

// Stop gracefully stops the scheduler and cancels running task contexts.
func (s *Scheduler) Stop() {
	s.cancel()
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// DefaultTasks returns the standard set of periodic tasks.
// Each task's Fn should be set by the caller with access to services.
func DefaultTasks() []Task {
	return []Task{
		{Name: "traffic-flush", Schedule: "0 */1 * * * *"},                              // every minute
		{Name: "quota-check", Schedule: "0 */5 * * * *"},                                // every 5 min
		{Name: "expiry-check", Schedule: "0 */5 * * * *"},                               // every 5 min
		{Name: "health-check", Schedule: "0 */5 * * * *"},                               // every 5 min
		{Name: "telegram-report", Schedule: "0 0 */6 * * *"},                            // every 6 hours
		{Name: "replication-sync", Schedule: "0 */1 * * * *", Timeout: 3 * time.Minute}, // SSH transfers can be slow
		{Name: "update-check", Schedule: "0 0 */6 * * *"},                               // every 6 hours
		{Name: "token-cleanup", Schedule: "0 0 */1 * * *"},                              // every hour
	}
}
