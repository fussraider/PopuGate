package scheduler

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/fussraider/PopuGate/pkg/logger"
)

var log = logger.WithScope("scheduler")

// Scheduler runs periodic tasks.
type Scheduler struct {
	cron *cron.Cron
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithSeconds()),
	}
}

// Task is a periodic task definition.
type Task struct {
	Name     string
	Schedule string // cron expression
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// DefaultTasks returns the standard set of periodic tasks.
// Each task's Fn should be set by the caller with access to services.
func DefaultTasks() []Task {
	return []Task{
		{Name: "traffic-flush", Schedule: "0 */1 * * * *"},        // every minute
		{Name: "quota-check", Schedule: "0 */5 * * * *"},          // every 5 min
		{Name: "expiry-check", Schedule: "0 */5 * * * *"},         // every 5 min
		{Name: "health-check", Schedule: "0 */5 * * * *"},         // every 5 min
		{Name: "telegram-report", Schedule: "0 0 */6 * * *"},      // every 6 hours
		{Name: "replication-sync", Schedule: "0 */1 * * * *"},     // every minute
		{Name: "update-check", Schedule: "0 0 */6 * * *"},         // every 6 hours
		{Name: "token-cleanup", Schedule: "0 0 */1 * * *"},        // every hour
	}
}
