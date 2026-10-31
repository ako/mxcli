// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Member references: the edges from a document to an ATTRIBUTE, an
// ASSOCIATION it navigates, an ENUMERATION it types something as, or an
// ENUMERATION_VALUE it names.
//
// Until these existed the reference graph stopped at documents, so
// `impact Module.Entity.Attr` answered "not referenced" for an attribute that a
// microflow writes and a page displays (Evora Factory Management:
// DigitalTwin.Machine.NumberOfIncidents). A missing edge is a wrong answer, not
// a missing feature — the tool states it confidently and the caller deletes.
//
// The sites are found by walking the RAW document, not the typed readers. A
// typed walk reaches the sites someone wrote a case for — change members,
// retrieve sorting — and silently misses the rest (aggregate-by-attribute,
// list operations, text template parameters, conditional visibility, pluggable
// widget attribute properties, mapping elements …). Every one of those stores
// the member by its fully qualified name, so matching every string value
// against the set of names the model actually declares reaches all of them with
// no per-type code. The match is on the WHOLE string (or on a whole path token
// inside an expression), against names that exist, so an unrelated string
// cannot produce an edge.
//
// What this cannot see: an attribute named only by its bare name through a
// variable in an expression (`$Order/Total`), because resolving `$Order` needs
// the variable's type. XPath is different — its context entity is known — and
// is handled by xpathRefs below.

// memberRefSourceTypes maps the unit types whose documents are walked to the
// catalog object type recorded as refs.SourceType. It is a closed list: the
// SourceType vocabulary is documented to lint-rule authors, and walking every
// unit would put values there (PAGE_TEMPLATE, BUILDING_BLOCK) that name
// design-time templates rather than anything that runs. Domain models are
// absent on purpose: an entity's own access rules and indexes name its members,
// and those are the entity describing itself, not another document using it.
var memberRefSourceTypes = map[string]string{
	"Microflows$Microflow":         RefObjectMicroflow,
	"Microflows$Nanoflow":          RefObjectNanoflow,
	"Microflows$Rule":              RefObjectRule,
	"Forms$Page":                   RefObjectPage,
	"Forms$Snippet":                RefObjectSnippet,
	"Workflows$Workflow":           RefObjectWorkflow,
	"ImportMappings$ImportMapping": RefObjectImportMapping,
	"ExportMappings$ExportMapping": RefObjectExportMapping,
	// A message definition exposes an entity, its attributes and associations
	// by qualified name (#1781). Before 11.15 the definitions sit inside a
	// collection document; from 11.15 each is a document of its own.
	"MessageDefinitions$MessageDefinitionCollection": RefObjectMessageDefinitionCollection,
	"MessageDefinitions$MessageDefinition2":          RefObjectMessageDefinition,
}

// memberRefSkipKeys are string properties that hold prose or a document's own
// name, never a reference. XPath constraints are skipped too: xpathRefs reads
// them with their context entity, which resolves bare attribute names the raw
// walk cannot.
var memberRefSkipKeys = map[string]bool{
	"Name":            true,
	"Documentation":   true,
	"XPathConstraint": true,
	"XpathConstraint": true,
}

// memberRefIndex is the set of names the model declares, which is what makes a
// string a reference rather than text.
type memberRefIndex struct {
	attributes     map[string]string // "Mod.Entity.Attr" -> enumeration QN, or "" when not an enumeration
	associations   map[string]bool
	enumerations   map[string]bool
	enumValues     map[string]bool // "Mod.Enum.Value"
	entities       map[string]bool
	generalization map[string]string // entity -> the entity it specializes

	// probe, when set, is told every name looked up, found or not: the
	// document being walked depends on it (builder_coverage.go).
	probe func(name string)
}

// read reports a lookup of name to the probe.
func (idx *memberRefIndex) read(name string) {
	if idx.probe != nil {
		idx.probe(name)
	}
}

// loadMemberRefIndex reads the name sets from the tables buildEntities,
// buildAssociations and buildEnumerations filled earlier in the transaction.
func (b *Builder) loadMemberRefIndex() *memberRefIndex {
	idx := &memberRefIndex{
		attributes:     map[string]string{},
		associations:   map[string]bool{},
		enumerations:   map[string]bool{},
		enumValues:     map[string]bool{},
		entities:       map[string]bool{},
		generalization: map[string]string{},
	}
	scan := func(query string, fn func(a, b string)) {
		rows, err := b.tx.Query(query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var a, c sql.NullString
			if rows.Scan(&a, &c) == nil && a.String != "" {
				fn(a.String, c.String)
			}
		}
	}
	scan(`SELECT EntityQualifiedName || '.' || Name, COALESCE(EnumerationQualifiedName, '') FROM attributes_data`,
		func(qn, enum string) { idx.attributes[qn] = enum })
	scan(`SELECT QualifiedName, '' FROM associations_data`,
		func(qn, _ string) { idx.associations[qn] = true })
	scan(`SELECT QualifiedName, '' FROM enumerations_data`,
		func(qn, _ string) { idx.enumerations[qn] = true })
	scan(`SELECT EnumerationQualifiedName || '.' || Name, '' FROM enumeration_values_data`,
		func(qn, _ string) { idx.enumValues[qn] = true })
	scan(`SELECT QualifiedName, COALESCE(Generalization, '') FROM entities_data`,
		func(qn, gen string) {
			idx.entities[qn] = true
			if gen != "" {
				idx.generalization[qn] = gen
			}
		})
	return idx
}

// resolveAttribute finds the attribute `name` on entity, or on the entity it
// specializes: an XPath over a specialization names inherited attributes
// bare, and the attribute's qualified name is the generalization's.
func (idx *memberRefIndex) resolveAttribute(entity, name string) (string, bool) {
	seen := map[string]bool{}
	for e := entity; e != "" && !seen[e]; e = idx.generalization[e] {
		seen[e] = true
		idx.read(e + "." + name)
		if _, ok := idx.attributes[e+"."+name]; ok {
			return e + "." + name, true
		}
	}
	return "", false
}

// refEdge is one outbound edge found in a document.
type refEdge struct {
	TargetType, TargetName, RefKind string
}

// edgeSet collects edges once each, in a deterministic order.
type edgeSet struct {
	seen  map[refEdge]bool
	edges []refEdge
}

func (s *edgeSet) add(targetType, targetName, kind string) {
	e := refEdge{targetType, targetName, kind}
	if s.seen == nil {
		s.seen = map[refEdge]bool{}
	}
	if !s.seen[e] {
		s.seen[e] = true
		s.edges = append(s.edges, e)
	}
}

func (s *edgeSet) sorted() []refEdge {
	sort.Slice(s.edges, func(i, j int) bool {
		a, b := s.edges[i], s.edges[j]
		if a.TargetType != b.TargetType {
			return a.TargetType < b.TargetType
		}
		if a.TargetName != b.TargetName {
			return a.TargetName < b.TargetName
		}
		return a.RefKind < b.RefKind
	})
	return s.edges
}

// memberRefsInUnit returns the member, enumeration and (for mappings and
// message definitions) entity edges one stored document makes.
func memberRefsInUnit(contents []byte, sourceType string, idx *memberRefIndex) []refEdge {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return nil
	}
	// Only these name an entity by a bare string that is a use of it; anywhere
	// else an entity name is matched by the typed reference extractors.
	isMapping := sourceType == RefObjectImportMapping || sourceType == RefObjectExportMapping ||
		sourceType == RefObjectMessageDefinitionCollection || sourceType == RefObjectMessageDefinition
	var set edgeSet
	walkBSONStrings(doc, "", func(key, v string) {
		if memberRefSkipKeys[key] || v == "" {
			return
		}
		// A whole-string match is a structured reference: MemberChange.Attribute,
		// AttributeRef.Attribute, EntityRefStep.Association, EnumerationType.Enumeration,
		// ObjectMappingElement.Entity.
		idx.read(v)
		switch {
		case hasKey(idx.attributes, v):
			set.add(RefObjectAttribute, v, RefKindMember)
			return
		case idx.associations[v]:
			set.add(RefObjectAssociation, v, RefKindMember)
			return
		case idx.enumerations[v]:
			set.add(RefObjectEnumeration, v, RefKindType)
			return
		case idx.enumValues[v]:
			set.add(RefObjectEnumerationValue, v, RefKindValue)
			return
		case isMapping && idx.entities[v]:
			set.add(RefObjectEntity, v, RefKindMapping)
			return
		}
		// Otherwise it may be an expression: enumeration values are always
		// written qualified, and an association path names its members.
		if strings.ContainsAny(v, "./") {
			scanPaths(v, "", idx, func(targetType, name string) {
				kind := RefKindMember
				if targetType == RefObjectEnumerationValue {
					kind = RefKindValue
				}
				set.add(targetType, name, kind)
			})
		}
	})
	return set.sorted()
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

// walkBSONStrings calls fn for every string property value in a decoded
// document, with the property's key. Arrays of strings are visited with the
// array's key.
func walkBSONStrings(v any, key string, fn func(key, value string)) {
	switch t := v.(type) {
	case string:
		fn(key, t)
	case bson.D:
		for _, e := range t {
			walkBSONStrings(e.Value, e.Key, fn)
		}
	case bson.M:
		for k, x := range t {
			walkBSONStrings(x, k, fn)
		}
	case map[string]any:
		for k, x := range t {
			walkBSONStrings(x, k, fn)
		}
	case bson.A:
		for _, x := range t {
			walkBSONStrings(x, key, fn)
		}
	case []any:
		for _, x := range t {
			walkBSONStrings(x, key, fn)
		}
	}
}

// xpathKeywords are XPath/expression words that can stand where an attribute
// name does and must never resolve to one.
var xpathKeywords = map[string]bool{
	"and": true, "or": true, "not": true, "true": true, "false": true,
	"empty": true, "div": true, "mod": true, "if": true, "then": true, "else": true,
}

// scanPaths finds the members an XPath constraint or an expression names.
//
// context is the entity bare names are resolved against — the retrieved entity
// for an XPath, "" for an expression (where a bare name hangs off a variable
// whose type is not known here). Inside a path, an entity segment becomes the
// context for the next one, and a predicate `[...]` after a path is evaluated
// against the path's last entity, so `Mod.Assoc/Mod.Other[Name = 'x']` resolves
// Name on Mod.Other.
//
// An enumeration attribute compared to a string literal (`Status = 'Open'`)
// names that enumeration value: XPath spells values as their bare name in
// quotes, so this is the only place an XPath reference to a value is visible.
func scanPaths(text, context string, idx *memberRefIndex, emit func(targetType, name string)) {
	stack := []string{context}
	top := func() string { return stack[len(stack)-1] }
	lastPathEntity := ""
	pendingEnum := "" // enumeration of the attribute just seen, until a comparison literal or anything else
	i := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == '\'':
			// String literal with '' escaping.
			j := i + 1
			var lit strings.Builder
			for j < len(text) {
				if text[j] == '\'' {
					if j+1 < len(text) && text[j+1] == '\'' {
						lit.WriteByte('\'')
						j += 2
						continue
					}
					break
				}
				lit.WriteByte(text[j])
				j++
			}
			if pendingEnum != "" {
				idx.read(pendingEnum + "." + lit.String())
			}
			if pendingEnum != "" && idx.enumValues[pendingEnum+"."+lit.String()] {
				emit(RefObjectEnumerationValue, pendingEnum+"."+lit.String())
			}
			pendingEnum = ""
			lastPathEntity = ""
			i = j + 1
		case c == '%':
			// XPath token such as '%CurrentDateTime%' outside quotes.
			j := strings.IndexByte(text[i+1:], '%')
			if j < 0 {
				return
			}
			pendingEnum = ""
			i += j + 2
		case c == '[':
			ctx := top()
			if lastPathEntity != "" {
				ctx = lastPathEntity
			}
			stack = append(stack, ctx)
			lastPathEntity = ""
			pendingEnum = ""
			i++
		case c == ']':
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			lastPathEntity = ""
			pendingEnum = ""
			i++
		case isIdentChar(c) || c == '$' || c == '.' || c == '/':
			j := i
			for j < len(text) && (isIdentChar(text[j]) || text[j] == '$' || text[j] == '.' || text[j] == '/') {
				j++
			}
			chunk := text[i:j]
			i = j
			// A function name (`contains(`, `toString(`) is not a member.
			k := j
			for k < len(text) && text[k] == ' ' {
				k++
			}
			if k < len(text) && text[k] == '(' {
				pendingEnum = ""
				lastPathEntity = ""
				continue
			}
			pendingEnum, lastPathEntity = resolvePath(chunk, top(), idx, emit)
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '=' || c == '!':
			i++
		default:
			pendingEnum = ""
			lastPathEntity = ""
			i++
		}
	}
}

// resolvePath resolves one navigation path such as
// `Mod.Assoc/Mod.Entity/Attr`, `$var/Mod.Assoc/Mod.Entity/Attr` or `Attr`,
// emitting the associations, attributes and enumeration values it names. It
// returns the enumeration of an attribute that ends the path (for a following
// `= 'Value'`) and the entity the path ends on (for a following predicate).
func resolvePath(chunk, context string, idx *memberRefIndex, emit func(targetType, name string)) (pendingEnum, endEntity string) {
	cur := context
	segs := strings.Split(chunk, "/")
	for n, seg := range segs {
		pendingEnum, endEntity = "", ""
		idx.read(seg)
		switch {
		case seg == "":
			cur = ""
		case strings.HasPrefix(seg, "$"):
			// A variable: its type is not known here, so what hangs off it
			// directly cannot be resolved. Qualified segments after it still can.
			cur = ""
		case idx.associations[seg]:
			emit(RefObjectAssociation, seg)
			cur = ""
		case idx.entities[seg]:
			cur = seg
			endEntity = seg
		case idx.enumValues[seg]:
			emit(RefObjectEnumerationValue, seg)
			cur = ""
		case !strings.Contains(seg, ".") && cur != "" && !xpathKeywords[seg]:
			if attr, ok := idx.resolveAttribute(cur, seg); ok {
				emit(RefObjectAttribute, attr)
				if n == len(segs)-1 {
					pendingEnum = idx.attributes[attr]
				}
			}
			cur = ""
		default:
			cur = ""
		}
	}
	return pendingEnum, endEntity
}

// extractMemberRefs walks every document of the types in memberRefSourceTypes
// and emits its member, enumeration and mapping-entity edges.
func (b *Builder) extractMemberRefs(stmt *Stmt, idx *memberRefIndex, projectID, snapshotID string) int {
	idx.probe = b.probe(siteMemberRefs)
	defer func() { idx.probe = nil }()
	unitTypes := make([]string, 0, len(memberRefSourceTypes))
	for t := range memberRefSourceTypes {
		unitTypes = append(unitTypes, t)
	}
	sort.Strings(unitTypes)

	count := 0
	for _, unitType := range unitTypes {
		sourceType := memberRefSourceTypes[unitType]
		units, err := b.lister().ListRawUnitsByType(unitType)
		if err != nil {
			continue
		}
		for _, u := range units {
			if !b.ownRows(string(u.ID)) {
				continue
			}
			// ListRawUnitsByType matches a PREFIX: Forms$Page also returns
			// Forms$PageTemplate units, which are not pages.
			if u.Type != unitType || len(u.Contents) == 0 {
				continue
			}
			var named struct {
				Name string `bson:"Name"`
			}
			if bson.Unmarshal(u.Contents, &named) != nil || named.Name == "" {
				continue
			}
			moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(u.ContainerID))
			sourceQN := moduleName + "." + named.Name
			for _, e := range memberRefsInUnit(u.Contents, sourceType, idx) {
				if _, err := stmt.Exec(sourceType, string(u.ID), sourceQN,
					e.TargetType, "", e.TargetName,
					e.RefKind, moduleName, projectID, snapshotID); err == nil {
					count++
				}
			}
		}
		b.disown()
	}
	return count
}

// extractEnumerationTypeRefs emits one `type` edge from every entity to each
// enumeration one of its attributes is typed as. Without it an enumeration had
// no inbound edge at all, and `impact` on it answered "not referenced".
func (b *Builder) extractEnumerationTypeRefs(projectID, snapshotID string) int {
	res, err := b.tx.Exec(
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		 SELECT DISTINCT ?, COALESCE(EntityId, ''), EntityQualifiedName, ?, '', EnumerationQualifiedName, ?, ModuleName, ?, ?
		 FROM attributes_data
		 WHERE EnumerationQualifiedName IS NOT NULL AND EnumerationQualifiedName != ''`,
		RefObjectEntity, RefObjectEnumeration, RefKindType, projectID, snapshotID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// xpathSourceTypes maps xpath_expressions_data.DocumentType to refs.SourceType.
// An access rule's constraint is recorded against the domain model with the
// entity as its qualified name; the entity is what depends on the attribute.
var xpathSourceTypes = map[string]string{
	"MICROFLOW":    RefObjectMicroflow,
	"NANOFLOW":     RefObjectNanoflow,
	"PAGE":         RefObjectPage,
	"SNIPPET":      RefObjectSnippet,
	"DOMAIN_MODEL": RefObjectEntity,
}

// extractXPathRefs emits `xpath` edges for the attributes, associations and
// enumeration values each recorded XPath constraint names, resolved against the
// constraint's target entity. buildXPathExpressions runs before buildReferences
// so the table is populated.
func (b *Builder) extractXPathRefs(stmt *Stmt, idx *memberRefIndex, projectID, snapshotID string) int {
	rows, err := b.tx.Query(`SELECT DocumentType, DocumentId, DocumentQualifiedName, COALESCE(TargetEntity, ''),
		XPathExpression, COALESCE(ModuleName, '') FROM xpath_expressions_data ORDER BY Id`)
	if err != nil {
		return 0
	}
	type xp struct{ docType, docID, docQN, target, xpath, module string }
	var all []xp
	for rows.Next() {
		var r xp
		if rows.Scan(&r.docType, &r.docID, &r.docQN, &r.target, &r.xpath, &r.module) == nil {
			all = append(all, r)
		}
	}
	rows.Close()

	count := 0
	written := map[string]bool{}
	for _, r := range all {
		sourceType, ok := xpathSourceTypes[r.docType]
		if !ok {
			continue
		}
		sourceID := r.docID
		if r.docType == "DOMAIN_MODEL" {
			sourceID = ""
		}
		var set edgeSet
		scanPaths(r.xpath, r.target, idx, func(targetType, name string) {
			set.add(targetType, name, RefKindXPath)
		})
		for _, e := range set.sorted() {
			key := sourceType + "\x00" + r.docQN + "\x00" + e.TargetType + "\x00" + e.TargetName
			if written[key] {
				continue
			}
			written[key] = true
			if _, err := stmt.Exec(sourceType, sourceID, r.docQN,
				e.TargetType, "", e.TargetName,
				e.RefKind, r.module, projectID, snapshotID); err == nil {
				count++
			}
		}
	}
	return count
}
