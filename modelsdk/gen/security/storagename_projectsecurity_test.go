// SPDX-License-Identifier: Apache-2.0

package security

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestProjectSecurityUserRoleKeysUseStorageNames pins the BSON keys of the two
// user-role references on Security$ProjectSecurity.
//
// The generator that produced this package bound both by their SDK names,
// "AdminUserRoleName" and "GuestUserRoleName". Mendix stores them without the
// suffix. The in-repo generator and real documents agree:
//
//	generated/metamodel/types.go
//	    AdminUserRoleName string `json:"adminUserRole,omitempty"`
//	    GuestUserRoleName string `json:"guestUserRole,omitempty"`
//	                    ^ SDK name       ^ storage name
//
//	A Studio Pro authored project's Security$ProjectSecurity unit:
//	    AdminUserRole   "Administrator"
//	    GuestUserRole   ""
//
// The guest one is what makes anonymous access authorable: mxbuild raises CE0133
// ("No user role for anonymous users selected even though the feature anonymous
// users is enabled") when EnableGuestAccess is on and this key is empty, so a
// value written under the wrong name is not a cosmetic slip — the app does not
// build. Reading the wrong key is quieter and was live: SHOW PROJECT SECURITY
// printed no guest role on the modelsdk engine while the legacy engine printed
// it, and the documented Starlark fields anonymous_user_role and
// role.is_anonymous resolved to "" / never for every project.
//
// A Primitive decodes and encodes under the same bound name (property.Primitive
// passes p.name to both), so one literal covers both directions — but assert
// both, because a future re-vendor of gen restores the SDK name in one place.
func TestProjectSecurityUserRoleKeysUseStorageNames(t *testing.T) {
	cases := []struct {
		storageKey string
		sdkName    string
		set        func(*ProjectSecurity, string)
		get        func(*ProjectSecurity) string
	}{
		{
			storageKey: "AdminUserRole",
			sdkName:    "AdminUserRoleName",
			set:        (*ProjectSecurity).SetAdminUserRoleName,
			get:        (*ProjectSecurity).AdminUserRoleName,
		},
		{
			storageKey: "GuestUserRole",
			sdkName:    "GuestUserRoleName",
			set:        (*ProjectSecurity).SetGuestUserRoleName,
			get:        (*ProjectSecurity).GuestUserRoleName,
		},
	}

	for _, tc := range cases {
		t.Run(tc.storageKey+"/encode", func(t *testing.T) {
			o := NewProjectSecurity()
			tc.set(o, "Administrator")

			var found bool
			for _, p := range o.Properties() {
				switch p.Name() {
				case tc.storageKey:
					found = true
				case tc.sdkName:
					t.Errorf("property is bound as %q — that is the SDK name, not the key on disk. "+
						"Writing it adds a property the type does not have, which Studio Pro refuses "+
						"to open (System.InvalidOperationException at MprProperty.cs) even though "+
						"mxbuild tolerates it", p.Name())
				}
			}
			if !found {
				t.Errorf("no %q property; this key is written to the .mxunit", tc.storageKey)
			}
		})

		t.Run(tc.storageKey+"/decode", func(t *testing.T) {
			raw, err := bson.Marshal(bson.D{
				{Key: "$Type", Value: "Security$ProjectSecurity"},
				{Key: tc.storageKey, Value: "Administrator"},
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			o := initProjectSecurity()
			o.InitFromRaw(bson.Raw(raw))

			if got := tc.get(o); got != "Administrator" {
				t.Errorf("decoded %q, want Administrator — InitFromRaw is reading the wrong key, "+
					"so every Studio Pro authored project reads back with no %s", got, tc.storageKey)
			}
		})
	}
}
