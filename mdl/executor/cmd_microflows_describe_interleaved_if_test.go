// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An `if` whose branches cross at two merges: the THEN arm goes straight to a
// shared region, and so does an arm two decisions deep in the ELSE; that region
// and another ELSE arm then meet at a second merge before the tail.
//
// Evora: OIDC.GetLoginEndpoint — with an IdP given, build its authorization
// URL; without one, look the configurations up, and when there are several
// but the IdP is still empty… a branch that also builds the authorization URL.
// Both URL-building paths, and the single-configuration one, then set the
// nonce cookie. DESCRIBE nested the inner arm's way to the shared region as
// "fall out of the if", which lands on the cookie and skips the URL: it
// carried the #923 "NOT equivalent" warning, correctly, because the branch
// structure was not the stored one.
const interleavedIfMDL = `create microflow M.InterleavedIf ($A: Boolean, $B: Boolean, $C: Boolean) returns Boolean
begin
  if $A then
    join first_path;
  else
    log info node 'T' 'lookup';
    if $B then
      if $C then
        log info node 'T' 'discover';
        return false;
      else
        join first_path;
      end if;
    else
      log info node 'T' 'default';
      join second_path;
    end if;
  end if;
  merge first_path;
  log info node 'T' 'auth';
  join second_path;
  merge second_path;
  log info node 'T' 'cookie';
  return true;
end;`

func TestDescribeInterleavedIf_EveryCrossingIsJoined(t *testing.T) {
	out, oc := describeBuilt(t, interleavedIfMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if strings.Contains(out, "NOT equivalent") {
		t.Errorf("a faithful description still carries the #923 refusal:\n%s", out)
	}
	for _, want := range []string{"'auth'", "'cookie'"} {
		if n := strings.Count(out, "node 'T' "+want); n != 1 {
			t.Errorf("%s is printed %d times, want once:\n%s", want, n, out)
		}
	}
}

// The textbook interleaving: two decisions, each of whose arms reaches both of
// two regions. No nesting of `if` places either region; merge/join does, one
// section each.
const fullyInterleavedIfMDL = `create microflow M.FullyInterleaved ($A: Boolean, $B: Boolean, $C: Boolean) returns Boolean
begin
  if $A then
    if $B then
      join d_path;
    else
      join e_path;
    end if;
  else
    if $C then
      join d_path;
    else
      join e_path;
    end if;
  end if;
  merge d_path;
  log info node 'T' 'd';
  join tail_path;
  merge e_path;
  log info node 'T' 'e';
  join tail_path;
  merge tail_path;
  return true;
end;`

func TestDescribeInterleavedIf_TwoDecisionsReachingTwoRegions(t *testing.T) {
	out, oc := describeBuilt(t, fullyInterleavedIfMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if strings.Contains(out, "NOT equivalent") {
		t.Errorf("a faithful description still carries the #923 refusal:\n%s", out)
	}
}
