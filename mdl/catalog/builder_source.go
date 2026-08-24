// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Object types collected into CATALOG.SOURCE. The executor owns the matching
// describe dispatch, in another package, so the two lists can drift — which is
// exactly what #912 was: nanoflows and rules were collected here and had no
// dispatch case, so every describe failed and both types contributed zero rows.
// TestDescribeDispatchCoversEverySourceObjectType pins the two together.
const (
	SourceEntity        = "ENTITY"
	SourceMicroflow     = "MICROFLOW"
	SourceNanoflow      = "NANOFLOW"
	SourceRule          = "RULE"
	SourcePage          = "PAGE"
	SourceSnippet       = "SNIPPET"
	SourceEnumeration   = "ENUMERATION"
	SourceWorkflow      = "WORKFLOW"
	SourceJsonStructure = "JSONSTRUCTURE"
	SourceImportMapping = "IMPORTMAPPING"
	SourceExportMapping = "EXPORTMAPPING"
)

// SourceObjectTypes is every type buildSource describes. Adding a collector
// without adding it here (and a dispatch case) fails the executor's coverage
// test rather than silently indexing nothing.
var SourceObjectTypes = []string{
	SourceEntity,
	SourceMicroflow,
	SourceNanoflow,
	SourceRule,
	SourcePage,
	SourceSnippet,
	SourceEnumeration,
	SourceWorkflow,
	SourceJsonStructure,
	SourceImportMapping,
	SourceExportMapping,
}

// sourceItem represents a single element to generate MDL source for.
type sourceItem struct {
	objType    string
	qn         string
	moduleName string
}

// sourceResult holds the output of a parallel describe call.
type sourceResult struct {
	item sourceItem
	text string
}

// describeFailure records a document that was collected but produced no row.
// Keeping these is the point: the pre-#912 code dropped them, so a whole
// document type could vanish from CATALOG.SOURCE without a word.
type describeFailure struct {
	item   sourceItem
	reason string
}

// buildSource generates full MDL source into the FTS5 source table.
// Only runs in source mode. Uses parallel workers for the describe calls
// since each is independent and CPU-bound.
func (b *Builder) buildSource() error {
	if !b.sourceMode {
		return nil
	}

	if b.describeFunc == nil {
		return nil
	}

	// Phase 1: Collect all items to describe (fast — uses cached lists)
	var items []sourceItem

	// Entities
	dms, err := b.cachedDomainModels()
	if err == nil {
		for _, dm := range dms {
			moduleID := b.hierarchy.findModuleID(dm.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			for _, ent := range dm.Entities {
				items = append(items, sourceItem{SourceEntity, moduleName + "." + ent.Name, moduleName})
			}
		}
	}

	// Microflows
	mfList, err := b.cachedMicroflows()
	if err == nil {
		for _, mf := range mfList {
			moduleID := b.hierarchy.findModuleID(mf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceMicroflow, moduleName + "." + mf.Name, moduleName})
		}
	}

	// Nanoflows
	nfList, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfList {
			moduleID := b.hierarchy.findModuleID(nf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceNanoflow, moduleName + "." + nf.Name, moduleName})
		}
	}

	// Rules. Without this a rule's body is not in CATALOG.SOURCE, so
	// `search '<text>'` cannot find an expression that only a rule contains.
	ruleList, err := b.cachedRules()
	if err == nil {
		for _, rule := range ruleList {
			moduleID := b.hierarchy.findModuleID(rule.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceRule, moduleName + "." + rule.Name, moduleName})
		}
	}

	// Pages
	pageList, err := b.cachedPages()
	if err == nil {
		for _, pg := range pageList {
			moduleID := b.hierarchy.findModuleID(pg.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourcePage, moduleName + "." + pg.Name, moduleName})
		}
	}

	// Snippets (not cached — only used here)
	snippetList, _ := b.reader.ListSnippets()
	for _, sn := range snippetList {
		moduleID := b.hierarchy.findModuleID(sn.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		items = append(items, sourceItem{SourceSnippet, moduleName + "." + sn.Name, moduleName})
	}

	// Workflows
	wfList, err := b.cachedWorkflows()
	if err == nil {
		for _, wf := range wfList {
			moduleID := b.hierarchy.findModuleID(wf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceWorkflow, moduleName + "." + wf.Name, moduleName})
		}
	}

	// Enumerations
	enumList, err := b.cachedEnumerations()
	if err == nil {
		for _, en := range enumList {
			moduleID := b.hierarchy.findModuleID(en.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceEnumeration, moduleName + "." + en.Name, moduleName})
		}
	}

	// JSON structures, import mappings and export mappings — the three types
	// #912 asked for. Each already had a catalog table and a Reader method; only
	// the source text was missing, so `search` could not reach a mapping's
	// element names or a structure's fields.
	jsonList, err := b.reader.ListJsonStructures()
	if err == nil {
		for _, js := range jsonList {
			moduleID := b.hierarchy.findModuleID(js.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceJsonStructure, moduleName + "." + js.Name, moduleName})
		}
	}

	importList, err := b.reader.ListImportMappings()
	if err == nil {
		for _, im := range importList {
			moduleID := b.hierarchy.findModuleID(im.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceImportMapping, moduleName + "." + im.Name, moduleName})
		}
	}

	exportList, err := b.reader.ListExportMappings()
	if err == nil {
		for _, em := range exportList {
			moduleID := b.hierarchy.findModuleID(em.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{SourceExportMapping, moduleName + "." + em.Name, moduleName})
		}
	}

	if len(items) == 0 {
		b.report("source", 0)
		return nil
	}

	// Phase 2: Generate MDL source in parallel. This is the slow phase on large
	// projects (one describe per document), so report progress periodically —
	// from a single ticker goroutine, the only caller of report() during the
	// run, so the progress sink isn't written concurrently.
	total := len(items)
	stop := make(chan struct{})
	var done atomic.Int64
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				b.report(fmt.Sprintf("source %d/%d documents", done.Load(), total), int(done.Load()))
			}
		}
	}()

	results, failures := runDescribes(items, b.describeFunc,
		max(min(runtime.NumCPU(), 8), 1),
		func(n int) { done.Store(int64(n)) })

	close(stop)

	// Phase 3: Insert results into FTS5 table (serial — SQLite constraint)
	stmt, err := b.tx.Prepare(`
		INSERT INTO source (QualifiedName, ObjectType, SourceText, ModuleName)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for _, res := range results {
		if res.text == "" {
			continue
		}
		stmt.Exec(res.item.qn, res.item.objType, res.text, res.item.moduleName)
		count++
	}

	b.report("source", count)

	// Report what produced no row. Silence here is what let #912 sit unnoticed:
	// 14 of 114 documents were dropped and the only visible number was the 100
	// that survived. Grouped by type, because a whole type failing is a wiring
	// bug while one document failing is a property of that document.
	if len(failures) > 0 {
		byType := map[string]int{}
		for _, f := range failures {
			byType[f.item.objType]++
		}
		for _, objType := range SourceObjectTypes {
			if n := byType[objType]; n > 0 {
				b.report(fmt.Sprintf("source: %d %s document(s) could not be described (e.g. %s)",
					n, objType, firstFailureReason(failures, objType)), n)
			}
		}
	}
	return nil
}

// firstFailureReason returns one representative reason for a type, so the
// report says why rather than only how many.
func firstFailureReason(failures []describeFailure, objType string) string {
	for _, f := range failures {
		if f.item.objType == objType {
			return f.item.qn + ": " + f.reason
		}
	}
	return ""
}

// runDescribes fans the describe calls out across workers and returns both the
// per-item results (positionally, so text stays matched to its document) and
// every item that produced no row.
//
// Split out of buildSource so the failure handling is testable without a
// project: the whole of #912 lived in the two swallowed error paths below.
func runDescribes(items []sourceItem, describe DescribeFunc, workers int, onProgress func(done int)) ([]sourceResult, []describeFailure) {
	if len(items) == 0 {
		return nil, nil
	}
	if workers < 1 {
		workers = 1
	}

	results := make([]sourceResult, len(items))
	failed := make([]*describeFailure, len(items))
	work := make(chan int, len(items))

	var done atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for idx := range work {
				item := items[idx]
				text, err := describe(item.objType, item.qn)
				switch {
				case err != nil:
					failed[idx] = &describeFailure{item, err.Error()}
				case text == "":
					failed[idx] = &describeFailure{item, "describe returned empty output"}
				default:
					results[idx] = sourceResult{item, text}
				}
				n := done.Add(1)
				if onProgress != nil {
					onProgress(int(n))
				}
			}
		})
	}

	for i := range items {
		work <- i
	}
	close(work)
	wg.Wait()

	// Collected in index order so the report is stable run to run.
	var failures []describeFailure
	for _, f := range failed {
		if f != nil {
			failures = append(failures, *f)
		}
	}
	return results, failures
}
