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

// TestFormAbort_ClearsFormAndReturnsCmd verifies that when a huh form is in
// StateAborted the Update path clears m.Form and returns a non-nil cmd
// (which is tea.ClearScreen to repaint).
func TestFormAbort_ClearsFormAndReturnsCmd(t *testing.T) {
	m := newTestModel(t)
	fwr := NewConfirmForm("Test?", "desc")
	fwr.Form.State = huh.StateAborted
	m.Form = &fwr
	m.formOnSubmit = func(interface{}) tea.Cmd { return nil }

	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := mNew.(Model)
	require.Nil(t, mm.Form, "Form should be cleared after abort")
	require.NotNil(t, cmd, "should return ClearScreen to repaint the tree")
}

// TestFormComplete_ClearsFormAndCallsCallback verifies that when a huh form
// transitions to StateCompleted, the form is cleared and the submit callback
// runs. A cmd (ClearScreen + optional action) is returned to repaint.
func TestFormComplete_ClearsFormAndCallsCallback(t *testing.T) {
	m := newTestModel(t)
	fwr := NewConfirmForm("Test?", "desc")
	fwr.Form.State = huh.StateCompleted
	m.Form = &fwr

	cbCalled := false
	m.formOnSubmit = func(interface{}) tea.Cmd {
		cbCalled = true
		return nil
	}

	mNew, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mNew.(Model)
	require.Nil(t, mm.Form, "Form should be nil after completion")
	require.True(t, cbCalled, "formOnSubmit callback should have been called")
	require.NotNil(t, cmd, "should return ClearScreen to repaint the tree")
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

// Direct exercise of the AccessibleReposLoadedMsg → form → cb path used by
// the 'a' picker on a host row. The user-visible bug was "adding a repo
// doesn't do anything"; these tests pin the closure logic that decides what
// happens after the form closes with each kind of selection.

func TestAddPicker_RealRepoSetsPendingAndDispatches(t *testing.T) {
	m := newTestModel(t)
	m.pickerHost = "h1"
	m.Hosts["h1"] = &poller.HostState{Name: "h1"}

	mNew, _ := m.Update(AccessibleReposLoadedMsg{Repos: []string{"acme/new"}})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "AccessibleReposLoadedMsg should open the form")
	require.NotNil(t, mm.formOnSubmit, "cb should be wired")

	cmd := mm.formOnSubmit([]string{"acme/new"})
	require.NotNil(t, cmd, "picking a real repo should return a non-nil cmd")
	require.Equal(t, "creating", mm.PendingPools["h1|acme/new"], "PendingPools should be set so the phantom shows")
}

func TestAddPicker_ManualSentinelOnlyDispatchesOpenMsg(t *testing.T) {
	m := newTestModel(t)
	m.pickerHost = "h1"
	m.Hosts["h1"] = &poller.HostState{Name: "h1"}

	mNew, _ := m.Update(AccessibleReposLoadedMsg{Repos: []string{"acme/foo"}})
	mm := mNew.(Model)
	cmd := mm.formOnSubmit([]string{manualEntrySentinel})
	require.NotNil(t, cmd, "picking only the sentinel should emit a cmd")
	msg := cmd()
	_, ok := msg.(openManualAddMsg)
	require.True(t, ok, "the cmd should emit openManualAddMsg, got %T", msg)
	require.Empty(t, mm.PendingPools, "manual-only pick should NOT set PendingPools (the input form will)")
}

func TestAddPicker_EmptySelectionFlashesHelp(t *testing.T) {
	m := newTestModel(t)
	m.pickerHost = "h1"
	m.Hosts["h1"] = &poller.HostState{Name: "h1"}

	mNew, _ := m.Update(AccessibleReposLoadedMsg{Repos: []string{"acme/foo"}})
	mm := mNew.(Model)
	cmd := mm.formOnSubmit([]string{})
	require.NotNil(t, cmd, "empty selection should emit a flash hint, not be a silent no-op")
	msg := cmd()
	fm, ok := msg.(flashMsg)
	require.True(t, ok, "should emit a flashMsg, got %T", msg)
	require.Contains(t, fm.Text, "toggle", "flash text should hint at the x/space toggle key")
}

func TestAddPicker_ManualPlusRealBatchesBoth(t *testing.T) {
	m := newTestModel(t)
	m.pickerHost = "h1"
	m.Hosts["h1"] = &poller.HostState{Name: "h1"}

	mNew, _ := m.Update(AccessibleReposLoadedMsg{Repos: []string{"acme/foo"}})
	mm := mNew.(Model)
	cmd := mm.formOnSubmit([]string{manualEntrySentinel, "acme/foo"})
	require.NotNil(t, cmd, "manual + real should still emit a cmd")
	require.Equal(t, "creating", mm.PendingPools["h1|acme/foo"], "real picks set PendingPools")
}

func TestOpenManualAddMsg_OpensInputForm(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(openManualAddMsg{host: "h1"})
	mm := mNew.(Model)
	require.NotNil(t, mm.Form, "openManualAddMsg should open the input form")
}
