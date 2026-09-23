// SPDX-License-Identifier: Apache-2.0

package types

// SnippetParameterTypeRule answers the one question both `mxcli check` and the
// snippet writer have to answer about a snippet parameter's declared type:
// may it be a primitive?
//
// It may not. Measured on Mendix 11.13.0 against a project whose only content
// was the snippets below (so nothing else could be the cause):
//
//	create snippet S.Label   ( params: { $Label: string } )
//	  → [error] [CE0046] "Invalid data type 'String'." at Snippet 'S.Label'
//	create snippet S.All     ( params: { $Label: String, $Count: Long,
//	                                     $Rank: Integer, $Amount: Decimal,
//	                                     $Active: Boolean, $Due: DateTime } )
//	  → one CE0046 per parameter, naming Studio Pro's caption for each
//	    ('String', 'Integer/Long' twice, 'Decimal', 'Boolean', 'Date and time')
//	create snippet S.Order   ( params: { $Order: S.Order } )
//	  → 0 errors
//
// And in the same run, the control that makes this a rule about SNIPPET
// parameters rather than about primitives: a PAGE declaring all six of those
// primitives as parameters builds at 0 errors.
//
// Storage does not encode the restriction — Forms$SnippetParameter's
// ParameterType is the polymorphic DataTypes$DataType, exactly as
// Forms$PageParameter's is (generated/metamodel: PagesSnippetParameter
// .ParameterType *DataTypesDataType), so a primitive serializes perfectly well.
// Only mxbuild's validator says no. That is why this has to live somewhere
// mxcli can consult it, and why a shape argument from the metamodel was not
// enough to settle it.
//
// Returned string is the Studio Pro caption mxbuild quotes in CE0046, so the
// message can be checked against a build log verbatim; "" means allowed.
func SnippetParameterTypeRule(bsonType string) (ce0046Caption string) {
	switch bsonType {
	case "DataTypes$StringType":
		return "String"
	case "DataTypes$IntegerType":
		// Studio Pro's single type for Integer and Long; storage has no LongType.
		return "Integer/Long"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$DateTimeType":
		return "Date and time"
	default:
		// "" is the entity case (no primitive $Type was resolved). An unmeasured
		// primitive is not guessed at — it falls through to mxbuild, which is
		// where the rule is actually enforced.
		return ""
	}
}
