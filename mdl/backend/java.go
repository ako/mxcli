// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// JavaBackend provides Java and JavaScript action operations.
type JavaBackend interface {
	ListJavaActions() ([]*types.JavaAction, error)
	ListJavaActionsFull() ([]*javaactions.JavaAction, error)
	// MoveJavaAction reparents a Java action to an already-updated ContainerID.
	MoveJavaAction(ja *javaactions.JavaAction) error
	ListJavaScriptActions() ([]*types.JavaScriptAction, error)
	ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error)
	ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error)
	CreateJavaAction(ja *javaactions.JavaAction) error
	UpdateJavaAction(ja *javaactions.JavaAction) error
	DeleteJavaAction(id model.ID) error
	// WriteJavaSourceFile writes javasource/<module>/actions/<Action>.java as
	// mxbuild generates it, documentation as the class javadoc. extraCode nil
	// keeps the file's EXTRA CODE section; anything else replaces it.
	WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType, extraImports []string, extraCode *string, documentation string) error
	DeleteJavaSourceFile(moduleName, actionName string) error
	RenameJavaSourceFile(moduleName, oldName, newName string) error
	// MoveJavaSourceFile moves an action's source to another module's actions
	// directory and rewrites its package line, refusing an occupied destination
	// before touching anything. It returns the new project-relative path, or ""
	// when there was no source to move.
	MoveJavaSourceFile(fromModule, toModule, actionName string) (string, error)
	ReadJavaSourceFile(moduleName, actionName string) (string, error)

	CreateJavaScriptAction(jsa *types.JavaScriptAction) error
	UpdateJavaScriptAction(jsa *types.JavaScriptAction) error
	DeleteJavaScriptAction(id model.ID) error
	WriteJavaScriptSourceFile(moduleName, actionName string, jsCode string, params []*types.JavaActionParameter, returnType types.CodeActionReturnType) error
	DeleteJavaScriptSourceFile(moduleName, actionName string) error
	RenameJavaScriptSourceFile(moduleName, oldName, newName string) error
	// MoveJavaScriptSourceFile is MoveJavaSourceFile for a JavaScript action;
	// its source names no module, so only the file moves.
	MoveJavaScriptSourceFile(fromModule, toModule, actionName string) (string, error)
}
