// internal/tui/poller/runners.go
package poller

import (
	"context"
	"time"

	"github.com/m-meyer2k/bobsled/internal/ghapp"
)

type RepoRunners struct {
	Runners []ghapp.RunnerRef
	ETag    string
	Updated time.Time
}

type RunnersMsg struct {
	Repo  string
	State *RepoRunners
	Err   error
}

// RunnersPoller polls each repo's runners endpoint with ETag conditional
// requests. One goroutine per repo.
func RunnersPoller(ctx context.Context, c *ghapp.Client, repos []string, interval time.Duration, emit chan<- RunnersMsg) {
	for _, r := range repos {
		go runnersLoop(ctx, c, r, interval, emit)
	}
	<-ctx.Done()
}

func runnersLoop(ctx context.Context, c *ghapp.Client, repo string, interval time.Duration, emit chan<- RunnersMsg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var etag string
	var lastList []ghapp.RunnerRef
	for {
		runners, newETag, err := c.ListRepoRunnersETag(ctx, repo, etag)
		msg := RunnersMsg{Repo: repo, Err: err}
		if err == nil {
			if runners == nil {
				msg.State = &RepoRunners{Runners: lastList, ETag: newETag, Updated: time.Now()}
			} else {
				lastList = runners
				etag = newETag
				msg.State = &RepoRunners{Runners: runners, ETag: newETag, Updated: time.Now()}
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
