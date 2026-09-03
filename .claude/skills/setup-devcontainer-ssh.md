# Set Up SSH Into Your Dev Container

A contributor workflow for getting an SSH key working so a **desktop app can open
a session in your dev container** — Claude Code Desktop's environment dropdown
has no "attach to container" option, so SSH is the only way in.

The machinery is already in the repo (`.devcontainer/ssh/`, wired to
`postStartCommand`). What is *not* in the repo, and cannot be, is your key
material: it is gitignored, so **every developer generates their own**. This
skill is the procedure for doing that and proving it works.

Reference for the setup itself — how the parts fit, and troubleshooting —
is [`.devcontainer/ssh/README.md`](../../.devcontainer/ssh/README.md). Do not
restate it here.

## When to Use This Skill

- First time you want to reach your dev container over SSH.
- `ssh` into the container fails and you need the elimination order.
- You are on a different host OS than the last person who touched this and the
  paths do not match.

Not needed for ordinary development — VS Code's own container attach is
unaffected by any of this.

## The Short Version

Inside the container:

```bash
bash .devcontainer/ssh/generate-key.sh   # once — your keypair, authorised
bash .devcontainer/ssh/start-sshd.sh     # postStartCommand also runs this
```

Both scripts are idempotent. `generate-key.sh` refuses to overwrite an existing
key (that would lock out anything already trusting it), and `start-sshd.sh`
merges into `authorized_keys` rather than replacing it.

Then, **on the host**, pre-trust the container and connect. Both values below are
per-developer — read them off your own run, do not copy them from a colleague or
from the README.

```bash
ssh-keyscan -p 2222 -t ed25519 localhost >> ~/.ssh/known_hosts
chmod 600 <repo>/.devcontainer/ssh/id_devcontainer
ssh -p 2222 -i <repo>/.devcontainer/ssh/id_devcontainer vscode@localhost
```

## Work Out Your Own Environment First

Three facts differ per machine. Establish them before debugging anything.

| Question | Command | Why it matters |
|---|---|---|
| Is the account password-locked? | `passwd -S "$(id -un)"` | `L` means pubkey is the **only** route in — don't chase password auth. Most devcontainer images lock it. |
| Is there a usable agent key? | `ssh-add -l` | `SSH_AUTH_SOCK` is usually forwarded but the agent is usually **empty**, so there is typically nothing to pull from the host. |
| Where is the workspace on the host? | `findmnt -T "$PWD" -o TARGET,SOURCE` | If it's a bind mount, a key generated *inside* the container is **already on your host** — no copying. `generate-key.sh` prints the derived host path. |

On Docker Desktop the `SOURCE` looks like
`/run/host_mark/Users[/you/GitHub/mxcli]`, which maps to `/Users/you/GitHub/mxcli`.
On a Linux host with a plain bind mount the derivation may not resolve — in that
case the host path is simply wherever you cloned the repo.

## Verify It, Don't Assume It

Run these in order. Each isolates one layer, and each has caught a real failure.

```bash
# 1. Does sshd accept the key at all? (inside the container)
ssh -i .devcontainer/ssh/id_devcontainer -p 2222 \
    -o BatchMode=yes -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/tmp/kh vscode@localhost 'echo OK'

# 2. Does it survive a rebuild? ~/.ssh is on the container's writable layer.
rm -f ~/.ssh/authorized_keys
bash .devcontainer/ssh/start-sshd.sh
test -f ~/.ssh/authorized_keys && echo restored

# 3. Does sshd serve the PINNED host key, not a fresh one?
diff <(ssh-keyscan -p 2222 -t ed25519 localhost 2>/dev/null | ssh-keygen -lf - | awk '{print $2}') \
     <(ssh-keygen -lf .devcontainer/ssh/hostkeys/ssh_host_ed25519_key.pub | awk '{print $2}') \
  && echo "pinned identity served"

# 4. The one that matters: a GUI client's exact conditions — no TTY, strict.
ssh -o StrictHostKeyChecking=yes -o BatchMode=yes -o UserKnownHostsFile=<file with only your line> \
    -i .devcontainer/ssh/id_devcontainer -p 2222 vscode@localhost 'echo OK'

# 5. Is the hardening actually enforced? Probe the LIVE daemon.
ssh -p 2222 -o PreferredAuthentications=password -o PubkeyAuthentication=no \
    vscode@localhost 2>&1 | tail -1
# want: Permission denied (publickey).
# bad:  Permission denied (publickey,password).   <- config not in effect
```

Check 4 is the meaningful one. A connection that works interactively can still
fail from a desktop app, because the app cannot answer *"Are you sure you want to
continue connecting?"*.

Check 5 catches a trap worth knowing about: **`sshd -T` is not evidence about the
running daemon.** It reads the config off disk; sshd freezes its config when the
listener starts. The sshd feature starts a listener from its entrypoint about a
second into container start, well before `postStartCommand` — so on a fresh
container the daemon can be enforcing the *feature's* config while `sshd -T`
happily reports ours. (Host keys are different: those are re-read per connection,
which is why pinning still works.) `start-sshd.sh` replaces the listener for this
reason, killing only the pid in `PidFile` so established sessions survive.

## Failure Modes

| Symptom | Cause | Fix |
|---|---|---|
| `Host key verification failed` + *"No ED25519 host key is known"* | The client has **no** entry and cannot prompt you to add one. | Append the `ssh-keyscan` line to `~/.ssh/known_hosts` on the host. |
| `Host key verification failed` naming an offending line | **Stale** entry — a previous container's identity. | `ssh-keygen -R '[localhost]:2222'`, then re-add. |
| `Permission denied (publickey)` | Key not in `authorized_keys`, or `~/.ssh` perms are loose. sshd **fails closed and silently** on loose perms. | Re-run `start-sshd.sh` (it sets `700`/`600`), then read the log — see below. |
| Connection refused from the host, works inside | Port not published. `forwardPorts` is editor-managed and only exists while VS Code is attached. | `appPort` must be set; rebuild the container. |
| `openssh-server is not installed` | The sshd feature didn't apply. | Rebuild the container. |
| Host key changed after an image rebuild | Host keys generated at **image build** belong to the image, not the container. | Expected only if `hostkeys/` was deleted; otherwise pinning prevents it. |
| `sshd -T` disagrees with how the daemon behaves | sshd froze its config at listener start; the feature started one before `postStartCommand`. | Re-run `start-sshd.sh` — it replaces the listener. Trust the probe in check 5, not `sshd -T`. |

## Reading sshd's Side of the Story

There is no syslog daemon in a container, so `/var/log/auth.log` does not exist
and sshd's account of a failure goes **nowhere**. Run it in the foreground:

```bash
sudo pkill -x sshd
sudo /usr/sbin/sshd -E /var/log/sshd.log -o LogLevel=VERBOSE
# reproduce, then:
sudo tail -50 /var/log/sshd.log
sudo pkill -x sshd && bash .devcontainer/ssh/start-sshd.sh   # restore
```

## Do Not

- **Do not commit key material.** Private user key, public key, and especially
  `hostkeys/` are gitignored. A committed private *host* key would let anyone
  with the repo impersonate a colleague's container to a client that trusts it.
  Confirm with `git check-ignore -v <path>` before any `git add -A`.
- **Do not share a key or a `known_hosts` line between developers.** Each
  container pins its own identity on first run; yours is not mine.
- **Do not swap `appPort` for `forwardPorts`** to "fix" a connection problem —
  it will work while VS Code is attached and fail exactly when you need it.
- **Do not bind the port to `0.0.0.0`** unless you mean to expose it to your
  LAN. The default is `127.0.0.1:2222:2222`.
