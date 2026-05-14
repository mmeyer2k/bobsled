// internal/tui/actions_test.go
package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdate_ActionResultErrorFlashes(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(ActionResultMsg{Err: assertErr("boom")})
	mm := mNew.(Model)
	require.NotNil(t, mm.Flash)
	require.True(t, mm.Flash.IsError)
	require.Contains(t, mm.Flash.Text, "boom")
}

func TestUpdate_ActionLogAppendsToBuffer(t *testing.T) {
	m := newTestModel(t)
	mNew, _ := m.Update(ActionLogMsg{Line: "hello"})
	mm := mNew.(Model)
	lines := mm.StatusLog.Lines()
	require.Equal(t, []string{"hello"}, lines)
}
