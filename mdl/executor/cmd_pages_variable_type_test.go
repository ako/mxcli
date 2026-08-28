// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// upstream #977 (part B): a page variable declared with an Enumeration type was
// stored as a String.
//
//	Variables: { $Filter: Enumeration(Repro6.ENUM_Status) = 'Repro6.ENUM_Status.Open' }
//
//	on disk:  VariableType: { $Type: "DataTypes$StringType" }
//	describe: Variables: { $Filter: String = '…' }
//
// Two silent defaults, stacked. mdlTypeToBsonType had no enumeration case and
// returned DataTypes$ObjectType for anything it did not recognise; then
// localVarTypeToGen had no ObjectType case and returned a StringType. Neither
// said anything, so a type nobody had mapped came out as "String" — which is
// also the reporter's second complaint, that a variable already typed Unknown
// reverts to String on the next page rewrite.
//
// The enumeration's qualified name was in the AST all along: the visitor stores
// the data type's raw source text, so `Enumeration(Mod.Enum)` arrives complete
// and only needed parsing.

func TestPageVariableType_ParsesAnEnumeration(t *testing.T) {
	cases := []struct{ in, wantType, wantEnum string }{
		{"Enumeration(Mod.Status)", "DataTypes$EnumerationType", "Mod.Status"},
		{"enumeration(Mod.Status)", "DataTypes$EnumerationType", "Mod.Status"},
		{"ENUMERATION( Mod.Status )", "DataTypes$EnumerationType", "Mod.Status"},
		{"Enum(Mod.Status)", "DataTypes$EnumerationType", "Mod.Status"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotType, gotEnum := pageVariableType(tc.in)
			if gotType != tc.wantType {
				t.Errorf("type = %q, want %q", gotType, tc.wantType)
			}
			if gotEnum != tc.wantEnum {
				t.Errorf("enumeration = %q, want %q — without it the stored type has no enumeration to point at",
					gotEnum, tc.wantEnum)
			}
		})
	}
}

// The controls: every type that already worked must map exactly as before, and
// carry no enumeration.
func TestPageVariableType_PrimitivesAreUnchanged(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Boolean", "DataTypes$BooleanType"},
		{"String", "DataTypes$StringType"},
		{"Integer", "DataTypes$IntegerType"},
		{"Long", "DataTypes$LongType"},
		{"Decimal", "DataTypes$DecimalType"},
		{"DateTime", "DataTypes$DateTimeType"},
		{"date", "DataTypes$DateTimeType"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotType, gotEnum := pageVariableType(tc.in)
			if gotType != tc.want {
				t.Errorf("type = %q, want %q", gotType, tc.want)
			}
			if gotEnum != "" {
				t.Errorf("enumeration = %q, want none", gotEnum)
			}
		})
	}
}

// An enumeration with no name in the parentheses cannot be pointed at, so it
// must not claim to be one — falling through keeps today's behaviour rather than
// writing an EnumerationType with nothing to resolve.
func TestPageVariableType_EnumerationWithoutAName(t *testing.T) {
	for _, in := range []string{"Enumeration()", "Enumeration", "Enumeration(   )"} {
		if got, enum := pageVariableType(in); got == "DataTypes$EnumerationType" || enum != "" {
			t.Errorf("%q produced %q/%q; an enumeration with no qualified name must not be written as one", in, got, enum)
		}
	}
}

// DESCRIBE has to render the stored type back, or the round trip loses it again
// at the other end.
func TestBsonTypeToMDLType_RendersAnEnumeration(t *testing.T) {
	if got, want := pageVariableMDLType("DataTypes$EnumerationType", "Mod.Status"), "Enumeration(Mod.Status)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Controls.
	if got, want := pageVariableMDLType("DataTypes$StringType", ""), "String"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// An EnumerationType whose reference did not survive is reported as Unknown
	// rather than as a bare `Enumeration(...)` that would not re-parse.
	if got := pageVariableMDLType("DataTypes$EnumerationType", ""); got == "Enumeration()" {
		t.Errorf("got %q — an enumeration with no name must not render as un-parseable MDL", got)
	}
}

// The pair, which is what the round trip actually rests on.
func TestPageVariableType_RoundTrips(t *testing.T) {
	for _, in := range []string{
		"Boolean", "String", "Integer", "Long", "Decimal", "DateTime",
		"Enumeration(Mod.Status)",
	} {
		bsonType, enumQN := pageVariableType(in)
		if got := pageVariableMDLType(bsonType, enumQN); got != in {
			t.Errorf("round trip of %q gave %q", in, got)
		}
	}
}
