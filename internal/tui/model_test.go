// internal/tui/model_test.go
package tui

import (
	"testing"
	"time"

	"github.com/m-meyer2k/bobsled/internal/inventory"
	"github.com/m-meyer2k/bobsled/internal/tui/poller"
	"github.com/stretchr/testify/require"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	inv := &inventory.Inventory{
		Hosts: map[string]inventory.Host{
			"h1": {SSH: "bobsled@h1", Capacity: 4},
		},
		Pools: []inventory.Pool{{Repo: "acme/foo", Count: 1, Spread: []string{"h1"}}},
	}
	return New(inv, nil, "inventory.yaml")
}

func TestUpdate_HostsTickStoresState(t *testing.T) {
	m := newTestModel(t)
	st := &poller.HostState{
		Name: "bobsled@h1", Reachable: true,
		Slots:      map[int]poller.SlotState{1: {N: 1, UnitState: "active", Repo: "acme/foo"}},
		LastUpdate: time.Now(),
	}
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "bobsled@h1", State: st}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Hosts["bobsled@h1"])
	require.Equal(t, "active", mm.Hosts["bobsled@h1"].Slots[1].UnitState)
}

func TestUpdate_RunnersTickStoresState(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(runnersTickMsg{M: poller.RunnersMsg{
		Repo:  "acme/foo",
		State: &poller.RepoRunners{},
	}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Runners["acme/foo"])
}

func TestUpdate_RecordsErrorBySource(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "bobsled@h1", Err: assertErr("boom")}})
	mm := mNew.(Model)
	require.Contains(t, mm.Errs["hosts/bobsled@h1"], "boom")
}

type stringErr string

func (e stringErr) Error() string { return string(e) }
func assertErr(s string) error    { return stringErr(s) }
