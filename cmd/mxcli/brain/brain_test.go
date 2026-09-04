// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var day = time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

func mustEntry(t *testing.T, text string, anchors ...string) Entry {
	t.Helper()
	e, err := NewEntry(text, anchors, day)
	if err != nil {
		t.Fatalf("NewEntry(%q): %v", text, err)
	}
	return e
}

func TestParseAnchor(t *testing.T) {
	for _, tc := range []struct {
		in                string
		ok                bool
		mod, elem, member string
	}{
		{"@Sales", true, "Sales", "", ""},
		{"@Sales.Order", true, "Sales", "Order", ""},
		{"@Sales.Order.Status", true, "Sales", "Order", "Status"},
		{"Sales.Order", true, "Sales", "Order", ""}, // the '@' is optional
		{"@Sales.Order.Status.Extra", false, "", "", ""},
		{"@9Sales", false, "", "", ""},
		{"@Sales..Order", false, "", "", ""},
		{"@", false, "", "", ""},
	} {
		a, err := ParseAnchor(tc.in)
		if tc.ok != (err == nil) {
			t.Errorf("ParseAnchor(%q): ok=%v, err=%v", tc.in, tc.ok, err)
			continue
		}
		if !tc.ok {
			continue
		}
		if a.Module != tc.mod || a.Element != tc.elem || a.Member != tc.member {
			t.Errorf("ParseAnchor(%q) = %+v", tc.in, a)
		}
	}
}

func TestShardIsDerivedFromTheFirstAnchor(t *testing.T) {
	e := mustEntry(t, "Orders are committed by Finance", "@Sales.Order", "@Finance.ACT_Post")
	if got := e.Shard(); got != "Sales" {
		t.Errorf("shard = %q, want Sales", got)
	}
	if got := mustEntry(t, "We deploy on Fridays").Shard(); got != ProjectShard {
		t.Errorf("anchorless shard = %q, want %q", got, ProjectShard)
	}
}

// The whole file format rests on this: what promote writes, check must be able
// to read back. A body containing text that looks like a heading or a metadata
// line is included on purpose.
func TestShardRoundTrips(t *testing.T) {
	in := []Entry{
		mustEntry(t, "Orders are committed by Finance\nNot by Sales, despite the entity living there.", "@Sales.Order", "@Finance.ACT_Post"),
		mustEntry(t, "No anchors here"),
		mustEntry(t, "Body with tricky text\nAnchors: not really a meta line\nand a - hyphen", "@Sales.Order"),
	}
	out, malformed, err := ParseShard("Sales", RenderShard("Sales", in))
	if err != nil {
		t.Fatal(err)
	}
	if len(malformed) != 0 {
		t.Fatalf("malformed blocks: %v", malformed)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d entries, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].ID != in[i].ID || out[i].Title != in[i].Title ||
			out[i].Body != in[i].Body || out[i].Date != in[i].Date ||
			strings.Join(out[i].Anchors, ",") != strings.Join(in[i].Anchors, ",") {
			t.Errorf("entry %d round-trip:\n got %+v\nwant %+v", i, out[i], in[i])
		}
	}
}

func TestMalformedEntryIsReportedNotSkipped(t *testing.T) {
	content := "# Sales\n\n" + shardMarker + "\n\n## A heading with no metadata line\n\nsome prose\n"
	entries, malformed, err := ParseShard("Sales", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
	if len(malformed) != 1 {
		t.Fatalf("malformed = %v, want 1 block reported", malformed)
	}
}

func TestPromoteRefusesPastTheCapAndAcceptsBelowIt(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}

	// Control: a first entry fits and is written.
	if err := s.Promote(mustEntry(t, "First"), ProjectShard); err != nil {
		t.Fatalf("first promote should fit: %v", err)
	}

	// Fill until the cap refuses, then assert nothing was written past it.
	var capErr *ErrCapExceeded
	for i := 0; i < 200; i++ {
		err := s.Promote(mustEntry(t, "Filler entry number "+string(rune('a'+i%26))+strings.Repeat("x", i)), ProjectShard)
		if err == nil {
			continue
		}
		var ok bool
		if capErr, ok = err.(*ErrCapExceeded); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
		break
	}
	if capErr == nil {
		t.Fatal("cap never bit; the budget is not being enforced")
	}
	b, err := os.ReadFile(s.ShardPath(ProjectShard))
	if err != nil {
		t.Fatal(err)
	}
	if got := CountLines(string(b)); got > ProjectShardCap {
		t.Errorf("shard is %d lines, past its %d cap — the refusal did not prevent the write", got, ProjectShardCap)
	}
}

func TestModuleShardGetsMoreRoomThanProject(t *testing.T) {
	if CapFor("Sales") <= CapFor(ProjectShard) {
		t.Fatal("project.md must be the tightest: it is the only file loaded unconditionally")
	}
}

func TestDropDeletesAnEmptiedModuleShardButKeepsProject(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	mod := mustEntry(t, "Only entry", "@Sales.Order")
	proj := mustEntry(t, "Only project entry")
	if err := s.Promote(mod, "Sales"); err != nil {
		t.Fatal(err)
	}
	if err := s.Promote(proj, ProjectShard); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.Drop(mod.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ShardPath("Sales")); !os.IsNotExist(err) {
		t.Error("an emptied module shard must be removed, not left as a husk")
	}

	// Control: emptying project.md leaves the file, because it is the store's
	// permanent home for cross-cutting facts.
	if _, _, err := s.Drop(proj.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ShardPath(ProjectShard)); err != nil {
		t.Errorf("project.md must survive being emptied: %v", err)
	}
}

func TestInitRefusesAForeignStoreAndAdoptsItsOwn(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "docs", "brain")
	if err := os.MkdirAll(foreign, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "README.md"), []byte("# Our team notes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir).Init(); err == nil {
		t.Fatal("init must refuse a docs/brain/ it did not write")
	}

	// Control: the same call on a store mxcli wrote is idempotent, not refused.
	own := t.TempDir()
	s := NewStore(own)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Init(); err != nil {
		t.Fatalf("re-init of mxcli's own store must succeed: %v", err)
	}
}

func TestInitNeverClobbersEntries(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	e := mustEntry(t, "Load-bearing decision")
	if err := s.Promote(e, ProjectShard); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	entries, _, err := s.LoadShard(ProjectShard)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("a second init discarded entries: %d left", len(entries))
	}
}

func TestQueueRefusesADuplicateButNotADifferentFact(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir)
	first := mustEntry(t, "Marketplace 4.5.0 broke the login flow", "@Administration.Account")
	added, err := q.Append(first)
	if err != nil || !added {
		t.Fatalf("first append: added=%v err=%v", added, err)
	}
	again := mustEntry(t, "marketplace 4.5.0   broke the LOGIN flow", "@Administration.Account")
	added, err = q.Append(again)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("the same fact captured twice must not queue twice")
	}
	// Control: a genuinely different fact is queued.
	added, err = q.Append(mustEntry(t, "Something else entirely"))
	if err != nil || !added {
		t.Fatalf("a different fact must queue: added=%v err=%v", added, err)
	}
	entries, err := q.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("queue has %d entries, want 2", len(entries))
	}
}

// stubResolver answers from a fixed table so the awkward states can be tested;
// a fixture project cannot readily produce a NotIndexable.
type stubResolver map[string]Resolution

func (s stubResolver) Resolve(a Anchor) (Resolution, error) {
	if r, ok := s[a.String()]; ok {
		return r, nil
	}
	return Resolution{State: NotFound}, nil
}

func TestCheckFailsOnMissingButNotOnNotIndexable(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	notIndexable := mustEntry(t, "Uses a document type the catalog does not index", "@Sales.SomeExoticDoc")
	if err := s.Promote(notIndexable, "Sales"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{"@Sales.SomeExoticDoc": {State: NotIndexable, Kind: "exotic"}}

	// A NotIndexable anchor cannot fail the check: the entry is current, the
	// index simply does not cover its target.
	rep, err := Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("NotIndexable must not fail the check: %+v", rep)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].State != NotIndexable {
		t.Errorf("NotIndexable must still be reported: %+v", rep.Findings)
	}

	// Control, same shape in the other direction: an anchor to nothing fails.
	gone := mustEntry(t, "Points at a deleted microflow", "@Sales.ACT_Gone")
	if err := s.Promote(gone, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() {
		t.Error("an anchor that names nothing must fail the check")
	}
}

func TestMisfilingIsASeparateAxisFromResolution(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Every anchor resolves perfectly; the entry is simply in the wrong file.
	e := mustEntry(t, "Filed under Sales but only mentions Finance", "@Finance.ACT_Post")
	if err := s.Promote(e, "Sales"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{"@Finance.ACT_Post": {State: Resolved, Module: "Finance", Kind: "microflow"}}
	rep, err := Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("no anchor problem expected: %+v", rep.Findings)
	}
	if len(rep.Misfiled) != 1 || rep.Misfiled[0].Belongs != "Finance" {
		t.Fatalf("expected one misfiled entry belonging to Finance: %+v", rep.Misfiled)
	}
	if !rep.Failed() {
		t.Error("a misfiled entry must fail the check")
	}
}

// The relaxation is deliberate: a two-module fact keeps its home shard as long
// as one anchor belongs there.
func TestCrossModuleEntryIsNotMisfiled(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	e := mustEntry(t, "Orders are committed by Finance", "@Sales.Order", "@Finance.ACT_Post")
	if err := s.Promote(e, "Sales"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{
		"@Sales.Order":      {State: Resolved, Module: "Sales", Kind: "entity"},
		"@Finance.ACT_Post": {State: Resolved, Module: "Finance", Kind: "microflow"},
	}
	rep, err := Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Misfiled) != 0 {
		t.Errorf("an anchor into another module must not misfile the entry: %+v", rep.Misfiled)
	}
	if rep.Failed() {
		t.Errorf("report should be clean: %+v", rep)
	}
}

func TestProjectShardIsNeverMisfiled(t *testing.T) {
	if MisfiledIn(ProjectShard, nil) {
		t.Error("the catch-all shard cannot be misfiled")
	}
}

func TestUsageIsComputedFromTheFilesThemselves(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.Promote(mustEntry(t, "One", "@Sales.Order"), "Sales"); err != nil {
		t.Fatal(err)
	}
	usage, err := s.Usage()
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, u := range usage {
		if u.Shard != "Sales" {
			continue
		}
		seen = true
		if u.Entries != 1 || u.Lines == 0 || u.Cap != ModuleShardCap {
			t.Errorf("unexpected usage: %+v", u)
		}
		if u.Headroom() != u.Cap-u.Lines {
			t.Errorf("headroom is not derived: %+v", u)
		}
	}
	if !seen {
		t.Fatalf("Sales shard missing from usage: %+v", usage)
	}
}

// The README must not carry a figure that promotion would falsify (A6).
func TestReadmeCarriesNoCountsOrSizes(t *testing.T) {
	readme := readmeContent()
	for _, bad := range []string{"entries)", "lines)", "currently", "so far"} {
		if strings.Contains(strings.ToLower(readme), bad) {
			t.Errorf("README states %q — computed figures must not be written down", bad)
		}
	}
}

// Regression: an entry whose only anchor is NotIndexable was reported as
// misfiled, because nothing had resolved to compare the shard against. That
// reintroduced A1's false-staleness signal through the other axis — the entry
// is perfectly current and the index simply does not cover its target.
func TestNotIndexableAnchorDoesNotMakeAnEntryLookMisfiled(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	e := mustEntry(t, "Anchored at a type the objects view skips", "@Sales.SomeExoticDoc")
	if err := s.Promote(e, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(s, stubResolver{"@Sales.SomeExoticDoc": {State: NotIndexable}}, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Misfiled) != 0 {
		t.Fatalf("misfiling is undecidable with nothing resolved: %+v", rep.Misfiled)
	}

	// Control: once one anchor resolves, misfiling is decidable again and this
	// same entry in the same shard IS reported.
	e2 := mustEntry(t, "Filed in Sales, resolves only in Finance", "@Finance.ACT_Post")
	if err := s.Promote(e2, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, stubResolver{
		"@Sales.SomeExoticDoc": {State: NotIndexable},
		"@Finance.ACT_Post":    {State: Resolved, Module: "Finance"},
	}, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Misfiled) != 1 || rep.Misfiled[0].EntryID != e2.ID {
		t.Fatalf("the resolvable entry must still be caught: %+v", rep.Misfiled)
	}
}

func mustRequirement(t *testing.T, text, slice string, anchors ...string) Entry {
	t.Helper()
	e, err := NewRequirement(text, anchors, slice, day)
	if err != nil {
		t.Fatalf("NewRequirement(%q): %v", text, err)
	}
	return e
}

// The central claim, with its control. A requirement's anchor points forward:
// not resolving means not built yet, which is the normal state. The identical
// entry recorded as a decision must still fail, or the distinction is doing
// nothing.
func TestUnbuiltRequirementDoesNotFailButTheSameDecisionDoes(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{} // resolves nothing

	req := mustRequirement(t, "Orders must be approvable", "02-approvals", "@Sales.ACT_Approve")
	if err := s.Promote(req, req.Shard()); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(s, r, []string{req.Shard()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("an unbuilt requirement must not fail the check: %+v", rep)
	}
	if len(rep.Slices) != 1 || rep.Slices[0].Planned != 1 || rep.Slices[0].Built != 0 {
		t.Fatalf("expected one planned requirement: %+v", rep.Slices)
	}

	// Control: the same sentence and anchor, recorded as a decision, fails.
	dec := mustEntry(t, "Orders must be approvable", "@Sales.ACT_Approve")
	if err := s.Promote(dec, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() {
		t.Error("a decision anchored at something missing must still fail — otherwise the kind distinction changes nothing")
	}
}

// Progress is derived from resolving anchors, so building the thing is what
// moves the number. Nothing in the file says "done".
func TestSliceProgressIsDerivedFromTheModel(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	built := mustRequirement(t, "Accounts can be created", "01-accounts", "@Admin.NewAccount")
	planned := mustRequirement(t, "Accounts can be archived", "01-accounts", "@Admin.ArchiveAccount")
	unanchored := mustRequirement(t, "It should feel fast", "01-accounts")
	for _, e := range []Entry{built, planned, unanchored} {
		if err := s.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}

	before := stubResolver{}
	rep, err := Check(s, before, []string{PlanShard("01-accounts")})
	if err != nil {
		t.Fatal(err)
	}
	got := rep.Slices[0]
	if got.Built != 0 || got.Planned != 2 || got.Unanchored != 1 {
		t.Fatalf("before: %+v", got)
	}

	// Build one of them — only the resolver changes, not the store.
	after := stubResolver{"@Admin.NewAccount": {State: Resolved, Module: "Admin", Kind: "microflow"}}
	rep, err = Check(s, after, []string{PlanShard("01-accounts")})
	if err != nil {
		t.Fatal(err)
	}
	got = rep.Slices[0]
	if got.Built != 1 || got.Planned != 1 || got.Unanchored != 1 {
		t.Fatalf("after: %+v — progress must follow the model, not the file", got)
	}
	if got.Total() != 3 {
		t.Errorf("total = %d, want 3", got.Total())
	}
}

// A requirement anchored at a document type the catalog does not index is
// BUILT: the thing exists. Counting it as planned would report finished work as
// outstanding — the same false signal as A1, in the progress report.
func TestNotIndexableCountsAsBuilt(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	e := mustRequirement(t, "A nightly job trims the audit log", "04-ops", "@Ops.NightlyTrim")
	if err := s.Promote(e, e.Shard()); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(s, stubResolver{"@Ops.NightlyTrim": {State: NotIndexable, Kind: "scheduled event"}},
		[]string{e.Shard()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Slices[0].Built != 1 {
		t.Fatalf("a not-indexable target exists and must count as built: %+v", rep.Slices[0])
	}
}

// A slice spans modules on purpose, so the misfiling rule that keeps decisions
// honest must not apply to it.
func TestCrossModuleSliceIsNeverMisfiled(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	e := mustRequirement(t, "Approving an order posts it to the ledger", "02-approvals",
		"@Sales.Order", "@Finance.ACT_Post")
	if err := s.Promote(e, e.Shard()); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(s, stubResolver{
		"@Sales.Order":      {State: Resolved, Module: "Sales"},
		"@Finance.ACT_Post": {State: Resolved, Module: "Finance"},
	}, []string{e.Shard()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Misfiled) != 0 {
		t.Fatalf("a slice spans modules by design: %+v", rep.Misfiled)
	}
}

// The same sentence can legitimately be a requirement of two slices; the id has
// to distinguish them or the second is refused as a duplicate.
func TestSameTextInTwoSlicesIsTwoRequirements(t *testing.T) {
	a := mustRequirement(t, "The list must paginate", "01-accounts")
	b := mustRequirement(t, "The list must paginate", "02-approvals")
	if a.ID == b.ID {
		t.Fatal("requirements in different slices must not collide")
	}
	// Control: the same text in the SAME slice is one requirement.
	c := mustRequirement(t, "The list must paginate", "01-accounts")
	if a.ID != c.ID {
		t.Fatal("the same requirement in the same slice must be one entry")
	}
}

func TestRequirementRoundTripsThroughItsSlice(t *testing.T) {
	in := []Entry{mustRequirement(t, "A title\nand a body", "02-approvals", "@Sales.Order")}
	shard := PlanShard("02-approvals")
	out, malformed, err := ParseShard(shard, RenderShard(shard, in))
	if err != nil || len(malformed) != 0 {
		t.Fatalf("err=%v malformed=%v", err, malformed)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries", len(out))
	}
	// The kind comes from the file, not from a second copy inside the entry.
	if out[0].EntryKind() != KindRequirement || out[0].Slice != "02-approvals" {
		t.Errorf("kind/slice not recovered from the shard: %+v", out[0])
	}
}

func TestSliceNamesAreValidated(t *testing.T) {
	for _, bad := range []string{"", "has space", "../escape", "a/b"} {
		if _, err := NewRequirement("x", nil, bad, day); err == nil {
			t.Errorf("slice %q should be refused", bad)
		}
	}
	for _, ok := range []string{"01-accounts", "approvals", "a_b", "2"} {
		if _, err := NewRequirement("x", nil, ok, day); err != nil {
			t.Errorf("slice %q should be accepted: %v", ok, err)
		}
	}
}

func TestPlanSlicesGetMoreRoomThanDecisions(t *testing.T) {
	if CapFor(PlanShard("01-x")) <= CapFor("Sales") {
		t.Fatal("a slice holds source material and is not loaded every session; it needs more room than a decision shard")
	}
}

func mustQuestion(t *testing.T, text, slice string, anchors ...string) Entry {
	t.Helper()
	e, err := NewQuestion(text, anchors, slice, day)
	if err != nil {
		t.Fatalf("NewQuestion(%q): %v", text, err)
	}
	return e
}

// A question's anchors are not checked, and the control is the identical
// anchor recorded as a decision. Often the question IS whether the thing should
// exist, so the staleness rule that keeps decisions honest would report every
// question as a defect.
func TestOpenQuestionIsNotCheckedButTheSameDecisionIs(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{} // resolves nothing

	q := mustQuestion(t, "Should approvers see rejected orders?", "", "@Sales.ACT_Nope")
	if err := s.Promote(q, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("an open question must not fail the check: %+v", rep)
	}
	if len(rep.Open) != 1 || rep.Open[0].EntryID != q.ID {
		t.Fatalf("the question must still be reported: %+v", rep.Open)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("a question's anchors must not be resolved at all: %+v", rep.Findings)
	}

	// Control: the same anchor as a settled decision still fails.
	d := mustEntry(t, "Approvers see rejected orders", "@Sales.ACT_Nope")
	if err := s.Promote(d, "Sales"); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, r, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() {
		t.Error("a decision anchored at nothing must still fail — otherwise --open disables the check for everything")
	}
}

// Resolution is the transition the whole kind exists for: the entry becomes a
// decision in place, keeps its identity, and its anchors start being checked.
func TestResolvingAQuestionMakesItACheckedDecision(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	q := mustQuestion(t, "Should approvers see rejected orders?", "", "@Sales.Order")
	if err := s.Promote(q, "Sales"); err != nil {
		t.Fatal(err)
	}
	resolved, err := q.Resolve("Yes, for 30 days", day)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != q.ID {
		t.Errorf("id changed on resolution (%s -> %s); an answered question is the same knowledge", q.ID, resolved.ID)
	}
	if resolved.Open {
		t.Error("a resolved question is no longer open")
	}
	if !strings.Contains(resolved.Body, "Should approvers see rejected orders?") {
		t.Errorf("the question must survive as the answer's context: %q", resolved.Body)
	}
	if err := s.Replace("Sales", resolved); err != nil {
		t.Fatal(err)
	}

	// Now it IS checked: a dead anchor on the answer fails.
	rep, err := Check(s, stubResolver{}, []string{"Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() {
		t.Error("once answered, the entry's anchors must be checked like any other decision")
	}
	if len(rep.Open) != 0 {
		t.Errorf("it is no longer an open question: %+v", rep.Open)
	}
}

func TestResolveRefusesSomethingThatIsNotAQuestion(t *testing.T) {
	if _, err := mustEntry(t, "A settled decision").Resolve("an answer", day); err == nil {
		t.Fatal("resolving a decision must be refused")
	}
	if _, err := mustQuestion(t, "A question", "").Resolve("", day); err == nil {
		t.Fatal("an empty answer must be refused")
	}
}

func TestReplaceKeepsPositionInTheShard(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	first := mustEntry(t, "First decision", "@Sales.A")
	q := mustQuestion(t, "A question?", "", "@Sales.B")
	last := mustEntry(t, "Last decision", "@Sales.C")
	for _, e := range []Entry{first, q, last} {
		if err := s.Promote(e, "Sales"); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := q.Resolve("An answer", day)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Replace("Sales", resolved); err != nil {
		t.Fatal(err)
	}
	entries, _, err := s.LoadShard("Sales")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[1].ID != q.ID || entries[1].Title != "An answer" {
		t.Fatalf("a resolved question must stay where it was, not move to the end: %+v", entries)
	}
}

// A question filed against a slice is not scope until it is answered, so it
// must not inflate the slice's outstanding work.
func TestSliceQuestionsAreCountedApartFromRequirements(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	req := mustRequirement(t, "Orders can be approved", "02-approvals", "@Sales.ACT_Approve")
	q := mustQuestion(t, "Do approvers see rejected orders?", "02-approvals", "@Sales.Order")
	for _, e := range []Entry{req, q} {
		if err := s.Promote(e, e.Shard()); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Check(s, stubResolver{}, []string{PlanShard("02-approvals")})
	if err != nil {
		t.Fatal(err)
	}
	got := rep.Slices[0]
	if got.Questions != 1 {
		t.Errorf("questions = %d, want 1: %+v", got.Questions, got)
	}
	if got.Planned != 1 || got.Total() != 1 {
		t.Errorf("an unanswered question is not outstanding scope: %+v", got)
	}
	if len(rep.Open) != 1 {
		t.Errorf("a slice's questions must still be reported: %+v", rep.Open)
	}
}

func TestOpenMarkerRoundTrips(t *testing.T) {
	in := []Entry{
		mustQuestion(t, "Still open?", "", "@Sales.Order"),
		mustEntry(t, "Settled", "@Sales.Order"),
	}
	out, malformed, err := ParseShard("Sales", RenderShard("Sales", in))
	if err != nil || len(malformed) != 0 {
		t.Fatalf("err=%v malformed=%v", err, malformed)
	}
	if !out[0].Open || out[1].Open {
		t.Fatalf("the OPEN marker did not round-trip: %+v", out)
	}
}

// Entries written before questions existed carry no marker and must read back
// as settled decisions, not as questions.
func TestEntryWithoutTheMarkerIsNotOpen(t *testing.T) {
	content := "# Sales\n\n" + shardMarker + "\n\n## An older entry\n\n" +
		"Anchors: `@Sales.Order` · id `abc123` · 2026-08-01\n"
	out, malformed, err := ParseShard("Sales", content)
	if err != nil || len(malformed) != 0 {
		t.Fatalf("err=%v malformed=%v", err, malformed)
	}
	if len(out) != 1 || out[0].Open {
		t.Fatalf("an entry with no marker is a settled decision: %+v", out)
	}
}
