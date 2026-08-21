// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

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
				items = append(items, sourceItem{"ENTITY", moduleName + "." + ent.Name, moduleName})
			}
		}
	}

	// Microflows
	mfList, err := b.cachedMicroflows()
	if err == nil {
		for _, mf := range mfList {
			moduleID := b.hierarchy.findModuleID(mf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"MICROFLOW", moduleName + "." + mf.Name, moduleName})
		}
	}

	// Nanoflows
	nfList, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfList {
			moduleID := b.hierarchy.findModuleID(nf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"NANOFLOW", moduleName + "." + nf.Name, moduleName})
		}
	}

	// Rules. Without this a rule's body is not in CATALOG.SOURCE, so
	// `search '<text>'` cannot find an expression that only a rule contains.
	ruleList, err := b.cachedRules()
	if err == nil {
		for _, rule := range ruleList {
			moduleID := b.hierarchy.findModuleID(rule.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"RULE", moduleName + "." + rule.Name, moduleName})
		}
	}

	// Pages
	pageList, err := b.cachedPages()
	if err == nil {
		for _, pg := range pageList {
			moduleID := b.hierarchy.findModuleID(pg.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"PAGE", moduleName + "." + pg.Name, moduleName})
		}
	}

	// Snippets (not cached — only used here)
	snippetList, _ := b.reader.ListSnippets()
	for _, sn := range snippetList {
		moduleID := b.hierarchy.findModuleID(sn.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		items = append(items, sourceItem{"SNIPPET", moduleName + "." + sn.Name, moduleName})
	}

	// Workflows
	wfList, err := b.cachedWorkflows()
	if err == nil {
		for _, wf := range wfList {
			moduleID := b.hierarchy.findModuleID(wf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"WORKFLOW", moduleName + "." + wf.Name, moduleName})
		}
	}

	// Enumerations
	enumList, err := b.cachedEnumerations()
	if err == nil {
		for _, en := range enumList {
			moduleID := b.hierarchy.findModuleID(en.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			items = append(items, sourceItem{"ENUMERATION", moduleName + "." + en.Name, moduleName})
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
	numWorkers := max(min(runtime.NumCPU(), 8), 1)

	results := make([]sourceResult, len(items))
	work := make(chan int, len(items))

	var done atomic.Int64
	total := len(items)
	stop := make(chan struct{})
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

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			for idx := range work {
				item := items[idx]
				text, err := b.describeFunc(item.objType, item.qn)
				if err == nil && text != "" {
					results[idx] = sourceResult{item, text}
				}
				done.Add(1)
			}
		})
	}

	for i := range items {
		work <- i
	}
	close(work)
	wg.Wait()
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
	return nil
}
