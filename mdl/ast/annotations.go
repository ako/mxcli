// SPDX-License-Identifier: Apache-2.0

package ast

import "reflect"

// StatementAnnotations returns the @-annotations attached to a microflow
// statement, or nil for a statement type that carries none.
//
// Read by reflection on purpose. Fifteen statement types have an `Annotations
// *ActivityAnnotations` field today, and a hand-written type switch over them
// would silently skip the sixteenth — which is exactly the failure this exists to
// catch, since the caller's job is to report annotations that were quietly
// dropped. TestEveryAnnotatedStatementIsReachable pins that the reflective read
// reaches every such type. (upstream #884)
func StatementAnnotations(s MicroflowStatement) *ActivityAnnotations {
	if s == nil {
		return nil
	}
	v := reflect.ValueOf(s)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("Annotations")
	if !f.IsValid() || f.Kind() != reflect.Ptr || f.IsNil() {
		return nil
	}
	ann, _ := f.Interface().(*ActivityAnnotations)
	return ann
}
