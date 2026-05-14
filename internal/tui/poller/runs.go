// internal/tui/poller/runs.go
package poller

import (
	"context"
	"sort"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
)

type RepoRuns struct {
	Queued     []ghapp.WorkflowRun
	InProgress []ghapp.WorkflowRun
	Recent     []ghapp.WorkflowRun
	ETag       string // composite (queued|in_progress|recent)
	Updated    time.Time
}

type RunsMsg struct {
	Repo  string
	State *RepoRuns
	Err   error
}

func RunsPoller(ctx context.Context, c *ghapp.Client, repos []string, interval time.Duration, emit chan<- RunsMsg) {
	for _, r := range repos {
		go runsLoop(ctx, c, r, interval, emit)
	}
	<-ctx.Done()
}

func runsLoop(ctx context.Context, c *ghapp.Client, repo string, interval time.Duration, emit chan<- RunsMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var (
		etagQ, etagP, etagR string
		lastQ, lastP, lastR []ghapp.WorkflowRun
	)
	for {
		queued, newQ, errQ := c.ListWorkflowRuns(ctx, repo, "queued", etagQ)
		inprog, newP, errP := c.ListWorkflowRuns(ctx, repo, "in_progress", etagP)
		recent, newR, errR := c.ListWorkflowRuns(ctx, repo, "", etagR)
		err := firstErr(errQ, errP, errR)
		if errQ == nil && queued == nil {
			queued = lastQ
		} else if errQ == nil {
			lastQ = queued
			etagQ = newQ
		}
		if errP == nil && inprog == nil {
			inprog = lastP
		} else if errP == nil {
			lastP = inprog
			etagP = newP
		}
		if errR == nil && recent == nil {
			recent = lastR
		} else if errR == nil {
			lastR = recent
			etagR = newR
		}
		sort.Slice(recent, func(i, j int) bool {
			return recent[i].RunStartedAt.After(recent[j].RunStartedAt)
		})
		msg := RunsMsg{Repo: repo, Err: err}
		if err == nil {
			msg.State = &RepoRuns{
				Queued: queued, InProgress: inprog, Recent: recent,
				ETag: etagQ + "|" + etagP + "|" + etagR, Updated: time.Now(),
			}
		}
		select {
		case <-ctx.Done():
			return
		case emit <- msg:
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
