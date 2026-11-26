package task

import (
	"path/filepath"
	"testing"
)

func TestMoveRecordsActorAndApprovalGate(t *testing.T) {
	b := &Board{}
	b.SetColumns(DevColumns)
	id := b.Add("x", "", nil, ref).ID
	b.Actor = "claude"
	if err := b.Move(id, StatusInDev, ref); err != nil {
		t.Fatal(err)
	}
	got, _ := b.TaskByID(id)
	if got.History[0].Agent != "claude" || got.LastAgent != "claude" {
		t.Fatalf("agent not recorded: %+v", got)
	}
	b.SetRequireApproval(true)
	if err := b.Move(id, StatusShipped, ref); err == nil {
		t.Fatal("agent shipped past the approval gate")
	}
	b.Actor = ""
	if err := b.Move(id, StatusShipped, ref); err != nil {
		t.Fatalf("human blocked by approval gate: %v", err)
	}
}

func TestBoardSettingsMergeFromOtherWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	a, _ := Load(path)
	a.SetColumns(DevColumns)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	other, _ := Load(path)
	other.SetWIPLimit(StatusInDev, 3)
	other.SetRequireApproval(true)
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	a.Add("y", "", nil, ref)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	final, _ := Load(path)
	if final.WIPLimits[StatusInDev] != 3 || !final.RequireApproval {
		t.Fatalf("settings lost by a concurrent writer: %+v %v", final.WIPLimits, final.RequireApproval)
	}
}
