// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// A cross-module MOVE JAVA ACTION reparented the document and left its source
// under the old module. mxbuild then built a stub that throws "Java action was
// not implemented" where the app calls the action, and compiled the author's
// code as an orphaned class (measured on 11.13.0). The move must take the source
// along — first, so an occupied destination refuses the statement before the
// model changes, and back again if the rest of the statement fails.

type sourceMoveProbe struct {
	calls     [][3]string // from, to, action
	unitMoved bool
}

func javaActionMoveBackend(t *testing.T, probe *sourceMoveProbe, moveUnitErr, moveSourceErr error) (*mock.MockBackend, *ContainerHierarchy, *model.Module, *model.Module) {
	t.Helper()
	srcMod, dstMod := mkModule("JamA"), mkModule("JamB")
	ja := &javaactions.JavaAction{Name: "JA_Hello"}
	ja.ID = nextID("ja")
	ja.ContainerID = srcMod.ID
	mb := &mock.MockBackend{
		IsConnectedFunc:         func() bool { return true },
		ListModulesFunc:         func() ([]*model.Module, error) { return []*model.Module{srcMod, dstMod}, nil },
		ListJavaActionsFullFunc: func() ([]*javaactions.JavaAction, error) { return []*javaactions.JavaAction{ja}, nil },
		MoveJavaActionFunc: func(*javaactions.JavaAction) error {
			if moveUnitErr != nil {
				return moveUnitErr
			}
			probe.unitMoved = true
			return nil
		},
		MoveJavaSourceFileFunc: func(from, to, action string) (string, error) {
			probe.calls = append(probe.calls, [3]string{from, to, action})
			if moveSourceErr != nil {
				return "", moveSourceErr
			}
			return "javasource/" + to + "/actions/" + action + ".java", nil
		},
		UpdateQualifiedNameInAllUnitsFunc: func(string, string) (int, error) { return 0, nil },
	}
	h := mkHierarchy(srcMod, dstMod)
	return mb, h, srcMod, dstMod
}

func TestMoveJavaActionAcrossModulesMovesItsSource(t *testing.T) {
	var probe sourceMoveProbe
	mb, h, _, _ := javaActionMoveBackend(t, &probe, nil, nil)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJavaAction,
		Name:         ast.QualifiedName{Module: "JamA", Name: "JA_Hello"},
		TargetModule: "JamB",
	}))
	if !probe.unitMoved {
		t.Fatal("the java action was not reparented")
	}
	if len(probe.calls) != 1 || probe.calls[0] != [3]string{"JamA", "JamB", "JA_Hello"} {
		t.Fatalf("source moves = %v, want one move JamA -> JamB of JA_Hello", probe.calls)
	}
}

// An occupied destination is refused before the document moves.
func TestMoveJavaActionRefusedBeforeTheModelChanges(t *testing.T) {
	var probe sourceMoveProbe
	mb, h, _, _ := javaActionMoveBackend(t, &probe, nil, errors.New("javasource/jamb/actions/JA_Hello.java already exists"))
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJavaAction,
		Name:         ast.QualifiedName{Module: "JamA", Name: "JA_Hello"},
		TargetModule: "JamB",
	})
	if err == nil {
		t.Fatal("want the move refused")
	}
	if probe.unitMoved {
		t.Error("the document moved although its source could not")
	}
}

// If the document cannot move, the source goes back where it was.
func TestMoveJavaActionPutsTheSourceBackOnFailure(t *testing.T) {
	var probe sourceMoveProbe
	mb, h, _, _ := javaActionMoveBackend(t, &probe, errors.New("disk full"), nil)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	if err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJavaAction,
		Name:         ast.QualifiedName{Module: "JamA", Name: "JA_Hello"},
		TargetModule: "JamB",
	}); err == nil {
		t.Fatal("want the move to fail")
	}
	want := [][3]string{{"JamA", "JamB", "JA_Hello"}, {"JamB", "JamA", "JA_Hello"}}
	if len(probe.calls) != 2 || probe.calls[0] != want[0] || probe.calls[1] != want[1] {
		t.Fatalf("source moves = %v, want %v", probe.calls, want)
	}
}

// A move within the module (into a folder) leaves the source where it is.
func TestMoveJavaActionWithinItsModuleLeavesTheSource(t *testing.T) {
	var probe sourceMoveProbe
	mb, h, _, _ := javaActionMoveBackend(t, &probe, nil, nil)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJavaAction,
		Name:         ast.QualifiedName{Module: "JamA", Name: "JA_Hello"},
	}))
	if len(probe.calls) != 0 {
		t.Errorf("source moved on a move within the module: %v", probe.calls)
	}
}
