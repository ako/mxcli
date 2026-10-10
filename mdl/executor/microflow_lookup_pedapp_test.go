// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The narrow microflow lookups decode only what they return; they must return
// exactly what filtering the full ListMicroflows gives, on Studio Pro-authored
// flows as well as mxcli's.
func TestMicroflowLookups_AgreeWithFullListing(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, redeclareChain(3, "create")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	ctx := exec.newExecContext(context.Background())
	if _, ok := ctx.Backend.(microflowLookup); !ok {
		t.Fatalf("backend %T has no narrow microflow lookup; the test would compare the fallback with itself", ctx.Backend)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil || len(all) < 4 {
		t.Fatalf("list: %d microflows, %v", len(all), err)
	}

	headers, err := microflowHeaders(ctx)
	if err != nil || len(headers) != len(all) {
		t.Fatalf("headers: %d for %d microflows, %v", len(headers), len(all), err)
	}
	for i, mf := range all {
		h := headers[i]
		if h.ID != mf.ID || h.ContainerID != mf.ContainerID || h.Name != mf.Name || h.Excluded != mf.Excluded {
			t.Errorf("header %d = %+v, microflow has %s %s %q excluded=%v", i, h, mf.ID, mf.ContainerID, mf.Name, mf.Excluded)
		}
	}

	for _, mf := range all {
		named, err := microflowsNamed(ctx, mf.Name)
		if err != nil {
			t.Fatalf("named %s: %v", mf.Name, err)
		}
		var want []*microflows.Microflow
		for _, m := range all {
			if m.Name == mf.Name {
				want = append(want, m)
			}
		}
		if !reflect.DeepEqual(named, want) {
			t.Errorf("microflowsNamed(%q) differs from the filtered full listing", mf.Name)
		}
		got, err := ctx.Backend.GetMicroflow(mf.ID)
		if err != nil || !reflect.DeepEqual(got, mf) {
			t.Errorf("GetMicroflow(%s) differs from the listed %s (%v)", mf.ID, mf.Name, err)
		}
	}
	if named, err := microflowsNamed(ctx, "NoSuchMicroflow"); err != nil || len(named) != 0 {
		t.Errorf("an unknown name returned %d microflows, %v", len(named), err)
	}
}
