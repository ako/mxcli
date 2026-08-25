// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ensuredb.go provisions the local PostgreSQL a standalone runtime needs, so a
// fresh session comes up testable without a manual createdb (slice 2 of the
// warm-loop proposal). It is best-effort: it starts the local Postgres service
// if the port is down — via a service manager, or a user-owned initdb/pg_ctl
// cluster when no service becomes ready (e.g. Arch, #823) — then ensures the app
// role and database exist via a superuser (a trust connection over the
// user-owned cluster's private socket, or `sudo -u postgres psql` for a system
// cluster). For a non-local DB host it does nothing but verify reachability —
// provisioning a remote database is not mxcli's business.

// pgIdent is a conservative PostgreSQL identifier (unquoted): a safe database or
// role name. We refuse anything else rather than quote/escape it into DDL.
var pgIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// serviceReadyTimeout bounds how long we probe for a service-manager-started
// server before falling back to a user-owned cluster. It is deliberately short:
// the single authoritative readiness wait lives in EnsureDatabase, so a dead or
// slow attempt must not multiply the overall deadline (#823 review). A variable
// so tests can shrink it further.
var serviceReadyTimeout = 3 * time.Second

// splitHostPort splits a PostgreSQL endpoint into host and port, defaulting the
// port to 5432 when absent. net.SplitHostPort handles the canonical bracketed
// IPv6 form; the compatibility branch keeps accepting the historical
// unbracketed "::1:5432" spelling used by this CLI.
func splitHostPort(hostPort string) (host, port string) {
	if host, port, err := net.SplitHostPort(hostPort); err == nil {
		return host, port
	}
	if strings.HasPrefix(hostPort, "[") && strings.HasSuffix(hostPort, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(hostPort, "["), "]"), "5432"
	}
	if strings.Count(hostPort, ":") > 1 {
		if i := strings.LastIndexByte(hostPort, ':'); i >= 0 {
			candidateHost, candidatePort := hostPort[:i], hostPort[i+1:]
			if net.ParseIP(candidateHost) != nil {
				if _, err := normalizePostgresPort(candidatePort); err == nil {
					return candidateHost, candidatePort
				}
			}
		}
		if net.ParseIP(hostPort) != nil {
			return hostPort, "5432"
		}
	}
	if i := strings.LastIndexByte(hostPort, ':'); i >= 0 {
		return hostPort[:i], hostPort[i+1:]
	}
	return hostPort, "5432"
}

// normalizePostgresPort validates a user-supplied PostgreSQL port and returns
// its canonical decimal form, so an invalid value cannot reach PostgreSQL's
// command arguments or managed configuration.
func normalizePostgresPort(port string) (string, error) {
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid PostgreSQL port %q (expected 1-65535)", port)
	}
	return strconv.FormatUint(n, 10), nil
}

// normalizePostgresEndpoint returns the endpoint in the single canonical form
// shared by psql, readiness checks, and the Mendix runtime configuration.
func normalizePostgresEndpoint(hostPort string) (host, port, endpoint string, err error) {
	host, port = splitHostPort(hostPort)
	port, err = normalizePostgresPort(port)
	if err != nil {
		return "", "", "", err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port, net.JoinHostPort(host, port), nil
}

// isLocalHost reports whether host refers to the local machine (so it is safe to
// start a service / use a local superuser).
func isLocalHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1", "":
		return true
	}
	return false
}

// quoteSQLString wraps s in single quotes for use as a SQL string literal,
// doubling any embedded single quotes (used only for the role password, which
// cannot be a bind parameter in CREATE ROLE).
func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// EnsureDatabase makes db reachable and the app role + database present. It is a
// no-op when the app can already connect. For a local host whose port is down it
// starts Postgres, then creates the role and database if missing. It
// canonicalizes db.Host in place so the runtime receives the exact endpoint
// that was checked and provisioned.
func EnsureDatabase(db *DBConfig, w io.Writer) error {
	if db == nil {
		return fmt.Errorf("--ensure-db requires a database configuration")
	}
	if strings.ToLower(db.Type) != "postgresql" {
		return fmt.Errorf("--ensure-db only supports PostgreSQL (DatabaseType=%q)", db.Type)
	}
	if !pgIdent.MatchString(db.Name) {
		return fmt.Errorf("unsafe database name %q (expected %s)", db.Name, pgIdent)
	}
	if !pgIdent.MatchString(db.User) {
		return fmt.Errorf("unsafe database user %q (expected %s)", db.User, pgIdent)
	}
	originalHost := db.Host
	host, port, endpoint, err := normalizePostgresEndpoint(originalHost)
	if err != nil {
		return fmt.Errorf("invalid --db-host %q: %w", originalHost, err)
	}
	db.Host = endpoint

	// Already usable? Then we're done.
	if canConnectDB(*db) {
		return nil
	}
	if !isLocalHost(host) {
		return fmt.Errorf("database is not usable at %s and host is not local; "+
			"start it and create the %q database (user %q)", db.Host, db.Name, db.User)
	}

	// Port down: start the local Postgres service (best-effort).
	if err := pingTCP(db.Host, 2*time.Second); err != nil {
		fmt.Fprintln(w, "  Starting local PostgreSQL...")
		if err := startLocalPostgres(host, port, w); err != nil {
			return fmt.Errorf("starting local PostgreSQL: %w", err)
		}
		if err := waitPGReady(host, port, 20*time.Second); err != nil {
			return err
		}
	}

	// Ensure the role and database exist (needs a local superuser).
	su, err := resolveSuperuser(host, port)
	if err != nil {
		return err
	}
	if err := ensureRole(su, *db, w); err != nil {
		return err
	}
	if err := ensureDatabase(su, *db, w); err != nil {
		return err
	}

	if !canConnectDB(*db) {
		return fmt.Errorf("provisioned PostgreSQL but still cannot connect to %q as %q at %s",
			db.Name, db.User, db.Host)
	}
	fmt.Fprintf(w, "  Database ready: %s (user %q) at %s\n", db.Name, db.User, db.Host)
	return nil
}

// canConnectDB reports whether the app can connect to its database as its user.
func canConnectDB(db DBConfig) bool {
	host, port := splitHostPort(db.Host)
	cmd := exec.Command("psql", "-X", "-w", "-h", host, "-p", port, "-U", db.User, "-d", db.Name,
		"-tAc", "select 1")
	cmd.Env = append(os.Environ(), "PGPASSWORD="+db.Password, "PGCONNECT_TIMEOUT=3")
	return cmd.Run() == nil
}

// startLocalPostgres starts the local PostgreSQL service, trying the common
// service managers in turn. When none is present — e.g. on Arch — or they do
// not produce a ready server, it falls back to a user-owned cluster started with
// the portable initdb/pg_ctl tools (#823).
func startLocalPostgres(host, port string, w io.Writer) error {
	var err error
	port, err = normalizePostgresPort(port)
	if err != nil {
		return err
	}
	// Only real, portable service managers belong here. The old
	// {"pg_ctlcluster", "--", "start"} entry was a placeholder whose args could
	// never start a cluster, so it only ever burned a readiness timeout before
	// the fallback — dropped (#823 review).
	attempts := [][]string{
		{"service", "postgresql", "start"},
	}
	var serviceDiag []string
	for _, a := range attempts {
		if _, err := exec.LookPath(a[0]); err != nil {
			continue
		}
		// The command may exit non-zero yet still bring Postgres up, so a short
		// readiness probe decides — not the exit code. The probe is intentionally
		// short: the single authoritative 20s wait is in EnsureDatabase, so a
		// slow-but-working manager is honoured there rather than paid for here.
		out, _ := exec.Command(a[0], a[1:]...).CombinedOutput()
		if waitPGReady(host, port, serviceReadyTimeout) == nil {
			return nil
		}
		if d := strings.TrimSpace(string(out)); d != "" {
			serviceDiag = append(serviceDiag, a[0]+": "+d)
		}
	}

	// A service-started server may own the port while crash recovery still makes
	// pg_isready report "not ready". Never initialize a competitor in that case:
	// EnsureDatabase's authoritative 20-second wait decides whether it recovers.
	// This guard also covers a process that won the port between the caller's
	// initial reachability check and this fallback.
	if pingTCP(net.JoinHostPort(host, port), time.Second) == nil {
		return nil
	}

	// No service manager made PostgreSQL ready: start a user-owned cluster with
	// the portable tools. This needs neither a `postgres` OS account nor sudo.
	if err := startUserCluster(host, port, w); err != nil {
		if len(serviceDiag) > 0 {
			// Surface what the service manager said; otherwise the user sees only
			// an initdb/pg_ctl error from two steps later.
			return fmt.Errorf("%w\n(a service manager ran first but Postgres did not "+
				"become ready:\n%s)", err, strings.Join(serviceDiag, "\n"))
		}
		return err
	}
	return nil
}

// userClusterDirs returns the state, data, and socket directories for the
// user-owned cluster under ~/.mxcli/postgres. The socket dir is separate and
// short because some systems cap the Unix-socket path length.
func userClusterDirs() (stateDir, dataDir, sockDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", fmt.Errorf("determining home directory: %w", err)
	}
	stateDir = filepath.Join(home, ".mxcli", "postgres")
	return stateDir, filepath.Join(stateDir, "data"), filepath.Join(stateDir, "sock"), nil
}

// writeFileAtomic replaces a cluster configuration file without exposing a
// partially-written file to a concurrent or subsequent postgres start.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keep = true
	return nil
}

func postgresConfigString(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("value contains a line break or NUL byte")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

const (
	mxcliPostgresConfigBegin = "# BEGIN mxcli managed connection settings"
	mxcliPostgresConfigEnd   = "# END mxcli managed connection settings"
)

// persistUserClusterConfig makes the private endpoint a property of the
// cluster, so it also holds for a later plain `pg_ctl -D ... start`.
func persistUserClusterConfig(dataDir, host, port, sockDir string) error {
	hostValue, err := postgresConfigString(host)
	if err != nil {
		return fmt.Errorf("quoting listen address: %w", err)
	}
	sockValue, err := postgresConfigString(sockDir)
	if err != nil {
		return fmt.Errorf("quoting socket directory: %w", err)
	}
	managed := fmt.Sprintf("%s\n"+
		"listen_addresses = %s\n"+
		"port = %s\n"+
		"unix_socket_directories = %s\n"+
		"unix_socket_permissions = 0700\n"+
		"%s\n", mxcliPostgresConfigBegin, hostValue, port, sockValue, mxcliPostgresConfigEnd)

	confPath := filepath.Join(dataDir, "postgresql.conf")
	conf, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("reading postgresql.conf: %w", err)
	}
	for {
		start := strings.Index(string(conf), mxcliPostgresConfigBegin)
		if start < 0 {
			break
		}
		relEnd := strings.Index(string(conf[start:]), mxcliPostgresConfigEnd)
		if relEnd < 0 {
			return fmt.Errorf("managed block in postgresql.conf is missing its end marker")
		}
		end := start + relEnd + len(mxcliPostgresConfigEnd)
		if end < len(conf) && conf[end] == '\n' {
			end++
		}
		conf = append(conf[:start], conf[end:]...)
	}
	conf = []byte(strings.TrimRight(string(conf), "\r\n"))
	if len(conf) > 0 {
		conf = append(conf, '\n', '\n')
	}
	conf = append(conf, managed...)
	if err := writeFileAtomic(confPath, conf, 0o600); err != nil {
		return fmt.Errorf("updating postgresql.conf: %w", err)
	}
	return nil
}

// rejectLegacyHostTrust prevents silent reuse of a cluster initialized by an
// earlier development revision, which used trust for loopback TCP. Local trust
// is expected and remains confined to the private Unix socket.
func rejectLegacyHostTrust(dataDir string) error {
	hbaPath := filepath.Join(dataDir, "pg_hba.conf")
	hba, err := os.ReadFile(hbaPath)
	if err != nil {
		return fmt.Errorf("reading pg_hba.conf: %w", err)
	}
	for lineNo, line := range strings.Split(string(hba), "\n") {
		fields := strings.Fields(strings.SplitN(line, "#", 2)[0])
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "host") {
			continue
		}
		for _, field := range fields[4:] {
			if field == "trust" {
				return fmt.Errorf("unsafe host trust authentication in %s line %d; stop PostgreSQL, "+
					"remove ~/.mxcli/postgres, and rerun --ensure-db", hbaPath, lineNo+1)
			}
		}
	}
	return nil
}

// clusterStatus reports whether a server is running from dataDir and, when
// readable, its TCP port and Unix-socket directory from postmaster.pid.
// `pg_ctl status` alone only proves that some server runs from this data directory.
func clusterStatus(dataDir string) (running bool, port, sockDir string) {
	if exec.Command("pg_ctl", "-D", dataDir, "status").Run() != nil {
		return false, "", ""
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "postmaster.pid"))
	if err != nil {
		return true, "", "" // running, but its endpoint is unknown
	}
	// postmaster.pid: line 1 PID, 2 data dir, 3 start time, 4 port, 5 socket dir…
	if lines := strings.Split(string(data), "\n"); len(lines) >= 5 {
		return true, strings.TrimSpace(lines[3]), strings.TrimSpace(lines[4])
	}
	return true, "", ""
}

// startUserCluster initializes (once) and starts a PostgreSQL cluster owned by
// the current user under ~/.mxcli/postgres, listening on host:port. It is safe
// to run repeatedly: an initialized data directory is reused and an already
// running server is left alone.
func startUserCluster(host, port string, w io.Writer) error {
	stateDir, dataDir, sockDir, err := userClusterDirs()
	if err != nil {
		return err
	}
	// Normalise an empty host so the persisted listen address is explicit even if
	// this helper is called directly with a bare ":port".
	if host == "" {
		host = "127.0.0.1"
	}
	if !isLocalHost(host) {
		return fmt.Errorf("refusing to start a user-owned PostgreSQL cluster on non-local host %q", host)
	}
	port, err = normalizePostgresPort(port)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		return fmt.Errorf("creating PostgreSQL socket directory %s: %w", sockDir, err)
	}
	// MkdirAll ignores its mode for an existing directory. Tighten it explicitly:
	// local trust authentication relies on this directory as its access boundary.
	if err := os.Chmod(sockDir, 0o700); err != nil {
		return fmt.Errorf("securing PostgreSQL socket directory %s: %w", sockDir, err)
	}

	// Initialize once — PG_VERSION marks a data directory initdb has populated.
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); os.IsNotExist(err) {
		fmt.Fprintln(w, "  Initializing user-owned PostgreSQL cluster...")
		// Bootstrap superuser "postgres". trust over the private 0700 socket keeps
		// our own provisioning password-free, but loopback TCP is scram-sha-256:
		// binding 127.0.0.1 is not an access control on a multi-user host, so trust
		// there would let any local account act as the postgres superuser.
		init := exec.Command("initdb", "-D", dataDir, "-U", "postgres",
			"--auth-local=trust", "--auth-host=scram-sha-256", "--encoding=UTF8")
		if out, err := init.CombinedOutput(); err != nil {
			return fmt.Errorf("initializing PostgreSQL cluster in %s: %w\n%s",
				dataDir, err, strings.TrimSpace(string(out)))
		}
	} else if err != nil {
		return fmt.Errorf("checking PostgreSQL cluster in %s: %w", dataDir, err)
	}

	if err := rejectLegacyHostTrust(dataDir); err != nil {
		return err
	}

	// A cluster left from an earlier --db-host satisfies `pg_ctl status` even when
	// its endpoint differs. Validate the live endpoint before changing its future
	// restart configuration, so a rejected request has no hidden side effect.
	running, livePort, liveSockDir := clusterStatus(dataDir)
	if running {
		if livePort == "" {
			return fmt.Errorf("a user-owned PostgreSQL cluster is running from %s, but its port "+
				"could not be read from %s", dataDir, filepath.Join(dataDir, "postmaster.pid"))
		}
		if livePort != port {
			return fmt.Errorf("a user-owned PostgreSQL cluster is already running from %s on "+
				"port %s, but port %s was requested — stop it with `pg_ctl -D %s stop` or rerun "+
				"with --db-host %s", dataDir, livePort, port, dataDir, net.JoinHostPort(host, livePort))
		}
		if liveSockDir == "" {
			return fmt.Errorf("a user-owned PostgreSQL cluster is running from %s, but its socket "+
				"directory could not be read from %s", dataDir, filepath.Join(dataDir, "postmaster.pid"))
		}
		if filepath.Clean(liveSockDir) != filepath.Clean(sockDir) {
			return fmt.Errorf("a user-owned PostgreSQL cluster is running with socket directory %s, "+
				"but the private directory %s is required — stop it with `pg_ctl -D %s stop` and "+
				"rerun --ensure-db", liveSockDir, sockDir, dataDir)
		}
	}

	if err := persistUserClusterConfig(dataDir, host, port, sockDir); err != nil {
		return fmt.Errorf("persisting PostgreSQL connection settings in %s: %w", dataDir, err)
	}
	if running {
		return nil
	}

	fmt.Fprintln(w, "  Starting user-owned PostgreSQL cluster...")
	logPath := filepath.Join(stateDir, "server.log")
	start := exec.Command("pg_ctl", "-D", dataDir, "-w",
		"-t", "30", "-l", logPath, "start")
	if out, err := start.CombinedOutput(); err != nil {
		return fmt.Errorf("starting PostgreSQL cluster in %s: %w\n%s\n  (see the server "+
			"log at %s)", dataDir, err, strings.TrimSpace(string(out)), logPath)
	}
	return nil
}

// waitPGReady polls pg_isready until the server accepts connections or timeout.
func waitPGReady(host, port string, timeout time.Duration) error {
	if _, err := exec.LookPath("pg_isready"); err != nil {
		// Fall back to a raw TCP check.
		return waitTCP(net.JoinHostPort(host, port), timeout)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("pg_isready", "-h", host, "-p", port)
		if cmd.Run() == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("PostgreSQL did not become ready at %s within %s", net.JoinHostPort(host, port), timeout)
}

// waitTCP polls a host:port until it accepts a connection or timeout.
func waitTCP(hostPort string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pingTCP(hostPort, time.Second) == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s did not accept connections within %s", hostPort, timeout)
}

// superuser is how we reach a PostgreSQL superuser to provision the role and
// database: a direct `psql -U postgres` against the user-owned cluster (over its
// private socket), or `sudo -u postgres psql` over the system cluster's default
// Unix socket. The latter preserves the peer-auth path used by Debian/Ubuntu.
type superuser struct {
	host, port string
	sock       string // Unix-socket dir for the user-owned cluster; preferred over TCP
	sudo       bool
}

// withoutPostgresTargetEnv prevents inherited libpq settings from overriding a
// deliberately host-less sudo connection. In that path, omitting -h selects the
// distribution's default Unix socket; an inherited PGHOST/PGHOSTADDR/PGSERVICE
// could otherwise silently turn it back into TCP or target another cluster.
func withoutPostgresTargetEnv(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "PGHOST", "PGHOSTADDR", "PGSERVICE":
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}

// psql builds a psql command for the superuser. ON_ERROR_STOP makes a failed
// statement a non-zero exit rather than a silent success; -X keeps a user's
// psqlrc from changing this non-interactive provisioning session.
func (s superuser) psql(args ...string) *exec.Cmd {
	// Prefer the cluster's private socket: initdb set --auth-local=trust there,
	// while loopback TCP is scram-sha-256, so the passwordless superuser can only
	// reach itself over the socket. A system-cluster sudo connection also needs a
	// Unix socket: changing the OS user to postgres enables peer auth, but gives no
	// TCP password. Its requested port still selects the exact default-socket
	// cluster. -w never prompts, so every other path fails fast.
	host := ""
	if s.sock != "" {
		host = s.sock
	} else if !s.sudo {
		host = s.host
	}
	base := []string{"-X", "-v", "ON_ERROR_STOP=1", "-w"}
	if host != "" {
		base = append(base, "-h", host)
	}
	base = append(base, "-p", s.port, "-U", "postgres", "-d", "postgres")
	if s.sudo {
		// -n prevents sudo from opening /dev/tty for a password; psql's -w above
		// independently prevents a database-password prompt. Omit -h to retain the
		// system cluster's peer-authenticated Unix socket, but pass the requested
		// port so a non-default cluster cannot fall through to port 5432.
		sudoBase := []string{"-n", "-u", "postgres", "--", "psql"}
		cmd := exec.Command("sudo", append(sudoBase, append(base, args...)...)...)
		cmd.Env = withoutPostgresTargetEnv(os.Environ())
		return cmd
	}
	return exec.Command("psql", append(base, args...)...)
}

// resolveSuperuser picks a working superuser path: a direct connection (the
// user-owned initdb cluster over its trust socket, or a plain loopback cluster —
// the only paths that work without elevation on Arch) or `sudo -u postgres` for
// a system cluster.
func resolveSuperuser(host, port string) (superuser, error) {
	if !isLocalHost(host) {
		return superuser{}, fmt.Errorf("refusing to use a local PostgreSQL superuser for non-local host %q", host)
	}
	// Prefer our cluster's private socket (trust); fall back to a plain
	// loopback connection for a pre-existing cluster a user pointed us at.
	candidates := []superuser{{host: host, port: port}}
	if _, _, sockDir, err := userClusterDirs(); err == nil {
		candidates = append([]superuser{{host: host, port: port, sock: sockDir}}, candidates...)
	}
	for _, direct := range candidates {
		if direct.psql("-tAc", "select 1").Run() == nil {
			return direct, nil
		}
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		sudo := superuser{host: host, port: port, sudo: true}
		if sudo.psql("-tAc", "select 1").Run() == nil {
			return sudo, nil
		}
	}
	return superuser{}, fmt.Errorf("no local PostgreSQL superuser available to create the " +
		"role/database (tried a direct 'psql -U postgres' connection over the cluster socket " +
		"and TCP, and non-interactive 'sudo -u postgres')")
}

// ensureRole creates the app login role if it does not already exist.
func ensureRole(su superuser, db DBConfig, w io.Writer) error {
	check := su.psql("-tAc", fmt.Sprintf("select 1 from pg_roles where rolname='%s'", db.User))
	out, _ := check.Output()
	if strings.TrimSpace(string(out)) == "1" {
		return nil
	}
	fmt.Fprintf(w, "  Creating role %q...\n", db.User)
	// initdb writes this setting when --auth-host=scram-sha-256 is used, but an
	// existing system cluster can still retain PostgreSQL <=13's md5 default.
	// Set it for this session so every role created for a SCRAM HBA record gets a
	// compatible verifier without changing the administrator's global config.
	ddl := fmt.Sprintf("SET standard_conforming_strings = on; SET password_encryption = 'scram-sha-256'; "+
		"CREATE ROLE %s WITH LOGIN PASSWORD %s CREATEDB;\n",
		db.User, quoteSQLString(db.Password))
	// Feed the only SQL containing a secret over stdin. Passing it via `-c` would
	// expose the database password in the process list to other local users.
	cmd := su.psql("-f", "-")
	cmd.Stdin = strings.NewReader(ddl)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating role %q: %w\n%s\n"+
			"  (need a local postgres superuser; create the role manually if unavailable)",
			db.User, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureDatabase creates the app database owned by the app role if it is absent.
func ensureDatabase(su superuser, db DBConfig, w io.Writer) error {
	check := su.psql("-tAc", fmt.Sprintf("select 1 from pg_database where datname='%s'", db.Name))
	out, _ := check.Output()
	if strings.TrimSpace(string(out)) == "1" {
		return nil
	}
	fmt.Fprintf(w, "  Creating database %q owned by %q...\n", db.Name, db.User)
	ddl := fmt.Sprintf("CREATE DATABASE %s OWNER %s", db.Name, db.User)
	cmd := su.psql("-c", ddl)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating database %q: %w\n%s", db.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
