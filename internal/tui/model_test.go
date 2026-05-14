// internal/tui/model_test.go
package tui

import (
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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
		Name: "h1", Reachable: true,
		Slots:      map[int]poller.SlotState{1: {N: 1, UnitState: "active", Repo: "acme/foo"}},
		LastUpdate: time.Now(),
	}
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "h1", State: st}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Hosts["h1"])
	require.Equal(t, "active", mm.Hosts["h1"].Slots[1].UnitState)
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
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "h1", Err: assertErr("boom")}})
	mm := mNew.(Model)
	require.Contains(t, mm.Errs["hosts/h1"], "boom")
}

func TestUpdate_HostsTickInitializesCursor(t *testing.T) {
	m := newTestModel(t)
	require.Equal(t, "", m.Cursor.Host, "cursor starts empty")
	st := &poller.HostState{
		Name: "h1", Reachable: true,
		Slots: map[int]poller.SlotState{1: {N: 1, UnitState: "active"}},
	}
	mNew, _ := m.Update(hostsTickMsg{M: poller.HostsMsg{Host: "h1", State: st}})
	mm := mNew.(Model)
	require.Equal(t, "h1", mm.Cursor.Host, "cursor parked on first host after tick")
	require.Equal(t, CursorHost, mm.Cursor.Kind)
}

// cmdYieldsMsg runs cmd() and returns the resulting tea.Msg.
func cmdYieldsMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// batchContainsForceRedraw inspects a tea.BatchMsg to see if any of its cmds
// produce a forceRedrawMsg. It also accepts a plain forceRedrawCmd.
func batchContainsForceRedraw(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if _, ok := msg.(forceRedrawMsg); ok {
		return true
	}
	// tea.BatchMsg is a []tea.Cmd under the covers; use reflection to walk it.
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice {
		return false
	}
	for i := range v.Len() {
		inner, ok := v.Index(i).Interface().(tea.Cmd)
		if !ok || inner == nil {
			continue
		}
		if _, ok2 := inner().(forceRedrawMsg); ok2 {
			return true
		}
	}
	return false
}

// TestFormAbort_EmitsForceRedraw verifies that when a huh form is in
// StateAborted the Update path clears m.Form and emits a forceRedrawCmd.
func TestFormAbort_EmitsForceRedraw(t *testing.T) {
	m := newTestModel(t)
	fwr := NewConfirmForm("Test?", "desc")
	// Force the form into StateAborted directly (mirroring the StateCompleted test).
	fwr.Form.State = huh.StateAborted
	m.Form = &fwr
	m.formOnSubmit = func(interface{}) tea.Cmd { return nil }

	// Any key triggers the switch on Form.State in Update.
	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := mNew.(Model)
	require.Nil(t, mm.Form, "Form should be cleared after abort")
	require.True(t, batchContainsForceRedraw(cmd), "forceRedrawMsg must be emitted on abort")
}

// TestFormComplete_EmitsForceRedraw verifies that when a huh form transitions
// to StateCompleted the Update path returns a forceRedrawCmd so Bubbletea
// re-renders the tree immediately (instead of waiting for the next key press).
func TestFormComplete_EmitsForceRedraw(t *testing.T) {
	m := newTestModel(t)
	fwr := NewConfirmForm("Test?", "desc")
	// Drive the form to StateCompleted by setting state directly.
	fwr.Form.State = huh.StateCompleted
	m.Form = &fwr

	cbCalled := false
	m.formOnSubmit = func(interface{}) tea.Cmd {
		cbCalled = true
		return nil // callback returns no further action
	}

	// Any key triggers the StateCompleted branch in Update.
	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mNew.(Model)
	require.Nil(t, mm.Form, "Form should be nil after completion")
	require.True(t, cbCalled, "formOnSubmit callback should have been called")
	require.True(t, batchContainsForceRedraw(cmd), "forceRedrawMsg must be emitted on completion")
}

// TestFormComplete_WithAction_BatchesAction checks the batch path where the
// callback returns a non-nil action cmd — it must be batched and the model
// must be cleared.
func TestFormComplete_WithAction_BatchesAction(t *testing.T) {
	m := newTestModel(t)
	fwr := NewConfirmForm("Test?", "desc")
	fwr.Form.State = huh.StateCompleted
	m.Form = &fwr

	called := false
	m.formOnSubmit = func(interface{}) tea.Cmd {
		called = true
		return func() tea.Msg { return struct{ name string }{"sentinel"} }
	}

	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mNew.(Model)
	require.Nil(t, mm.Form, "Form should be nil after completion")
	require.NotNil(t, cmd, "should return a cmd (Batch of action + ClearScreen)")
	require.True(t, called, "the submit callback ran")
}

// TestForceRedrawMsg_IsNoOp verifies that Update handles forceRedrawMsg without
// side-effects — it exists only to trigger a render cycle.
func TestForceRedrawMsg_IsNoOp(t *testing.T) {
	m := newTestModel(t)
	m.Cursor.Host = "h1" // set something so we can check it's unchanged
	mNew, cmd := m.Update(forceRedrawMsg{})
	mm := mNew.(Model)
	require.Equal(t, "h1", mm.Cursor.Host, "forceRedrawMsg should not mutate model")
	require.Nil(t, cmd, "forceRedrawMsg handler should return nil cmd")
}

type stringErr string

func (e stringErr) Error() string { return string(e) }
func assertErr(s string) error    { return stringErr(s) }
