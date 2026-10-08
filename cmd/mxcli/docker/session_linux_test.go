// SPDX-License-Identifier: Apache-2.0

//go:build linux

package docker

import (
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestParseProcStat_CommWithSpacesAndParens(t *testing.T) {
	line := "4242 (my (odd) proc) S 1 4242 4200 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 3 0 987654 1000 10 18446744073709551615"
	st, ok := parseProcStat(line)
	if !ok || st.state != "S" || st.session != 4200 || st.startTicks != 987654 {
		t.Fatalf("got %+v ok=%v", st, ok)
	}
	if _, ok := parseProcStat("garbage"); ok {
		t.Error("garbage parsed")
	}
}

// A session leader's grandchild stays in the session after the leader is
// gone — which is exactly the orphan `run stop` has to find.
func TestSessionMembers_FindsOrphanedGrandchild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & echo started; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, _ := cmd.StdoutPipe()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sh: %v", err)
	}
	buf := make([]byte, 16)
	_, _ = out.Read(buf)
	leader := cmd.Process.Pid
	members := SessionMembers(leader, start)
	if !slices.Contains(members, leader) || len(members) < 2 {
		t.Fatalf("members of session %d = %v, want the leader and its sleep", leader, members)
	}
	_ = syscall.Kill(leader, syscall.SIGKILL)
	_ = cmd.Wait()
	if PidAlive(leader) {
		t.Fatal("leader reaped but still alive")
	}
	left := SessionMembers(leader, start)
	if len(left) != 1 {
		t.Fatalf("after the leader died, members = %v; want the orphaned sleep", left)
	}
	_ = syscall.Kill(left[0], syscall.SIGKILL)

	// The start-time guard: a session observed with a notBefore after its
	// processes started is treated as someone else's (pid reuse).
	if got := SessionMembers(leader, time.Now().Add(time.Hour)); len(got) != 0 {
		t.Errorf("start-time guard let %v through", got)
	}
}

// The pid-reuse guard rebuilds a start time from btime (whole seconds, so up
// to 1s early) plus starttime (10ms ticks, so up to 10ms early). A process
// that started just after notBefore must still count as this run's: with only
// 1s of slack it fell out whenever the two truncations added up to more,
// which made SessionMembers return nothing for a live run — `run stop`
// reporting "was not running", and three tests here failing on CI a few
// percent of the time.
func TestStartedBefore_ToleratesBtimeAndTickTruncation(t *testing.T) {
	realBoot := time.Unix(1000, 999_900_000) // btime reads 1000
	btime := time.Unix(1000, 0)
	// A start 9.9ms into a tick, 1ms after notBefore: truncation takes off
	// 0.9999s + 9.9ms, more than the old 1s of slack plus the 1ms gap.
	notBefore := realBoot.Add(50*time.Second + 8900*time.Microsecond)
	actualStart := notBefore.Add(time.Millisecond)
	ticks := int64(actualStart.Sub(realBoot) / (time.Second / clockTicks)) // truncated, as the kernel does
	if startedBefore(btime, ticks, notBefore) {
		t.Errorf("a process started %v after notBefore was taken for an earlier one (ticks %d)", actualStart.Sub(notBefore), ticks)
	}
	// The guard still does its job: a session that began long before is not this run's.
	if !startedBefore(btime, ticks, notBefore.Add(time.Hour)) {
		t.Error("a process from an hour before notBefore passed the guard")
	}
}
