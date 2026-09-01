// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/tunnelhub/audit"
)

// sessionCookieName is the SSO cookie shared across every *.<CookieDomain> preview.
const sessionCookieName = "mxcli_hub_session"

// defaultSessionTTL is how long a session cookie is valid before a silent
// re-auth (the GitHub session makes the redirect invisible while still valid).
const defaultSessionTTL = 12 * time.Hour

// AuthConfig holds the GitHub OAuth + session-cookie configuration for the hub.
// The zero value (empty GitHubClientID) is **open mode**: the middleware is a
// no-op, viewer resolution returns "" (everyone sees everything), and
// registration keeps honouring the legacy X-Hub-Secret — i.e. today's behaviour.
type AuthConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	SessionSecret      []byte        // HMAC key for the session cookie
	CookieDomain       string        // e.g. ".mxcli.org" for SSO across subdomains
	HubHost            string        // the hub host (for the OAuth callback URL)
	RequireAuth        bool          // gate previews + register on a valid session
	SessionTTL         time.Duration // 0 → defaultSessionTTL

	Audit audit.Sink // where auth events go; nil → audit.NoOp()

	// Overridable for tests (default to the real GitHub endpoints / clock).
	githubAPIBase   string       // default "https://api.github.com"
	githubOAuthBase string       // default "https://github.com"
	httpClient      *http.Client // default http.DefaultClient
	now             func() time.Time
}

// enabled reports whether auth is configured (a GitHub client id is present).
// When false the hub runs in open mode.
func (a *AuthConfig) enabled() bool {
	return a != nil && a.GitHubClientID != ""
}

func (a *AuthConfig) clock() time.Time {
	if a != nil && a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *AuthConfig) ttl() time.Duration {
	if a != nil && a.SessionTTL > 0 {
		return a.SessionTTL
	}
	return defaultSessionTTL
}

func (a *AuthConfig) auditSink() audit.Sink {
	if a != nil && a.Audit != nil {
		return a.Audit
	}
	return audit.NoOp()
}

// Signing tags domain-separate the two token types so a session cookie can never
// be replayed as an OAuth state (or vice versa): the tag is mixed into the MAC,
// and each verifier only accepts its own tag.
const (
	tagSession = "session"
	tagState   = "state"
)

// signTagged produces an HMAC-signed value carrying {msg, exp} under a domain tag.
// Format: base64url(payload) "." base64url(HMAC-SHA256(tag ‖ 0x00 ‖ payload)),
// payload = "msg|expUnix". The tag is authenticated but not carried in the token
// (the verifier supplies the expected tag).
func signTagged(secret []byte, tag, msg string, exp time.Time) string {
	payload := msg + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(tag))
	mac.Write([]byte{0})
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

// verifyTagged validates a value under the expected tag and returns msg when the
// signature is valid and it has not expired. Constant-time signature comparison.
func verifyTagged(secret []byte, tag, value string, now time.Time) (msg string, ok bool) {
	dot := strings.LastIndexByte(value, '.')
	if dot <= 0 {
		return "", false
	}
	payloadB64, sigB64 := value[:dot], value[dot+1:]
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(tag))
	mac.Write([]byte{0})
	mac.Write(payload)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sigB64)) {
		return "", false
	}
	// payload = "msg|expUnix"; msg (GitHub login / b64 url) never contains '|'.
	sep := strings.LastIndexByte(string(payload), '|')
	if sep <= 0 {
		return "", false
	}
	expUnix, err := strconv.ParseInt(string(payload[sep+1:]), 10, 64)
	if err != nil {
		return "", false
	}
	if now.Unix() > expUnix {
		return "", false // expired
	}
	return string(payload[:sep]), true
}

// signSession / verifySession are the session-cookie wrappers (tag "session").
func signSession(secret []byte, login string, exp time.Time) string {
	return signTagged(secret, tagSession, login, exp)
}

func verifySession(secret []byte, value string, now time.Time) (login string, ok bool) {
	return verifyTagged(secret, tagSession, value, now)
}

// setSessionCookie writes a fresh signed session cookie for login.
func (a *AuthConfig) setSessionCookie(w http.ResponseWriter, login string) {
	exp := a.clock().Add(a.ttl())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signSession(a.SessionSecret, login, exp),
		Path:     "/",
		Domain:   a.CookieDomain,
		Expires:  exp,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie (logout).
func (a *AuthConfig) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   a.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
	})
}

// sessionLogin returns the authenticated GitHub login from the request's session
// cookie, or "" when auth is off or the cookie is absent/invalid/expired.
func (a *AuthConfig) sessionLogin(r *http.Request) string {
	if !a.enabled() {
		return ""
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	login, ok := verifySession(a.SessionSecret, c.Value, a.clock())
	if !ok {
		return ""
	}
	return login
}

func (a *AuthConfig) client() *http.Client {
	if a != nil && a.httpClient != nil {
		return a.httpClient
	}
	return http.DefaultClient
}

func (a *AuthConfig) oauthBase() string {
	if a != nil && a.githubOAuthBase != "" {
		return a.githubOAuthBase
	}
	return "https://github.com"
}

func (a *AuthConfig) apiBase() string {
	if a != nil && a.githubAPIBase != "" {
		return a.githubAPIBase
	}
	return "https://api.github.com"
}

func (a *AuthConfig) callbackURL() string {
	return "https://" + a.HubHost + "/auth/github/callback"
}

// signState signs an OAuth `state` carrying the post-login return URL (base64 so
// no delimiter collides), reusing the session HMAC scheme with a short TTL.
func (a *AuthConfig) signState(returnURL string) string {
	enc := base64.RawURLEncoding.EncodeToString([]byte(returnURL))
	return signTagged(a.SessionSecret, tagState, enc, a.clock().Add(10*time.Minute))
}

func (a *AuthConfig) verifyState(state string) (returnURL string, ok bool) {
	enc, ok := verifyTagged(a.SessionSecret, tagState, state, a.clock())
	if !ok {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// clientIP returns the source address for audit. The hub terminates TLS directly
// (it *is* the 443 edge), so RemoteAddr is the real peer and X-Forwarded-For is
// attacker-controlled — we deliberately do NOT trust it, to keep audit IPs
// unspoofable. A deployment that fronts the hub with a trusted reverse proxy would
// need an explicit allow-listed-proxy option before honouring XFF.
func clientIP(r *http.Request) string {
	return stripPort(r.RemoteAddr)
}

// authHandler returns the /auth/* mux (login, callback, logout). Only mounted
// when auth is enabled.
func (a *AuthConfig) authHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/github/login", a.handleLogin)
	mux.HandleFunc("/auth/github/callback", a.handleCallback)
	mux.HandleFunc("/auth/logout", a.handleLogout)
	return mux
}

// handleLogin (GET /auth/github/login?return=<url>) 302s to GitHub's authorize
// endpoint with a signed state carrying the return URL.
func (a *AuthConfig) handleLogin(w http.ResponseWriter, r *http.Request) {
	ret := r.URL.Query().Get("return")
	if ret == "" {
		ret = "https://" + a.HubHost + "/"
	}
	q := url.Values{
		"client_id":    {a.GitHubClientID},
		"redirect_uri": {a.callbackURL()},
		"scope":        {"read:user"},
		"state":        {a.signState(ret)},
	}
	http.Redirect(w, r, a.oauthBase()+"/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

// handleCallback (GET /auth/github/callback) validates state, exchanges the code
// for a token, learns the login, sets the SSO session cookie, and 302s back.
func (a *AuthConfig) handleCallback(w http.ResponseWriter, r *http.Request) {
	audit := a.auditSink()
	ret, ok := a.verifyState(r.URL.Query().Get("state"))
	if !ok {
		audit.Log(auditEventFail(r, "", "invalid state"))
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		audit.Log(auditEventFail(r, "", "missing code"))
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	token, err := a.exchangeCode(code)
	if err != nil {
		audit.Log(auditEventFail(r, "", "code exchange failed"))
		http.Error(w, "authentication failed", http.StatusBadGateway)
		return
	}
	login, err := a.fetchLogin(token)
	if err != nil || login == "" {
		audit.Log(auditEventFail(r, "", "user lookup failed"))
		http.Error(w, "authentication failed", http.StatusBadGateway)
		return
	}
	a.setSessionCookie(w, login)
	audit.Log(auditEvent(r, auditLoginOK, login))
	// Only redirect back to our own hosts (defence against open-redirect).
	if !a.isValidRedirect(ret) {
		http.Redirect(w, r, "https://"+a.HubHost+"/", http.StatusFound)
		return
	}
	http.Redirect(w, r, ret, http.StatusFound)
}

// handleLogout (POST /auth/logout) clears the session cookie.
func (a *AuthConfig) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	login := a.sessionLogin(r)
	a.clearSessionCookie(w)
	a.auditSink().Log(auditEvent(r, audit.EventLogout, login))
	w.WriteHeader(http.StatusNoContent)
}

// isValidRedirect allows redirecting only to the hub host or a subdomain of the cookie
// domain, so a crafted `return` can't bounce the browser off-site after login.
//
// The NAME is load-bearing and so is the shape of its use in handleCallback.
// CodeQL's go/unvalidated-url-redirection only credits a redirect validator whose
// callee name matches (?i)(is_?)?(local_?url|valid_?redir(ect)?)(ur[li])?, and only
// sanitizes reads the guard's true-branch dominates. Measured on 2.20.3, all four
// combinations: this name + early return is the ONLY one that clears the alert --
// renaming it back, or going back to reassigning `ret` and falling through to a
// single Redirect, each independently make the (already-fixed) alert reappear.
func (a *AuthConfig) isValidRedirect(ret string) bool {
	u, err := url.Parse(ret)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := stripPort(u.Host)
	if h == a.HubHost {
		return true
	}
	if a.CookieDomain == "" {
		return false
	}
	// Compare against the registrable domain, not the operator's spelling of it.
	// A bare strings.HasSuffix is a substring test, not a subdomain test: with
	// CookieDomain "mxcli.org" it also accepts "evil-mxcli.org", which anyone can
	// register. buildHubAuth's default supplies the leading dot that makes the
	// naive form correct, but --cookie-domain takes an arbitrary string and
	// nothing tells the operator the dot is load-bearing. Normalising here means
	// both spellings behave identically and the flag cannot be mis-typed into an
	// open redirect out of the OAuth callback -- which fires AFTER the session
	// cookie is set, so the victim lands off-site already authenticated.
	domain := strings.TrimPrefix(a.CookieDomain, ".")
	return h == domain || strings.HasSuffix(h, "."+domain)
}

func (a *AuthConfig) exchangeCode(code string) (string, error) {
	q := url.Values{
		"client_id":     {a.GitHubClientID},
		"client_secret": {a.GitHubClientSecret},
		"code":          {code},
		"redirect_uri":  {a.callbackURL()},
	}
	req, _ := http.NewRequest(http.MethodPost, a.oauthBase()+"/login/oauth/access_token", strings.NewReader(q.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github token exchange returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return "", err
	}
	if body.Error != "" {
		return "", fmt.Errorf("github token exchange failed: %s", body.Error)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("github token exchange returned no access_token")
	}
	return body.AccessToken, nil
}

func (a *AuthConfig) fetchLogin(token string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, a.apiBase()+"/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := a.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github /user returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return "", err
	}
	return body.Login, nil
}

// audit event constants aliased for brevity in this file.
const (
	auditLoginOK = audit.EventLoginOK
)

func auditEvent(r *http.Request, event, login string) audit.Event {
	return audit.Event{Event: event, Login: login, IP: clientIP(r), Outcome: "ok"}
}

func auditEventFail(r *http.Request, login, detail string) audit.Event {
	return audit.Event{Event: audit.EventCallbackFail, Login: login, IP: clientIP(r), Outcome: "fail", Detail: detail}
}

// authorizePreview enforces the owner check on a preview request. It returns true
// when the request may proceed; otherwise it has already written the response
// (302 to login when unauthenticated, 403 when the viewer isn't the owner) and
// the caller must stop. It only enforces when auth is enabled AND RequireAuth is
// set: in open mode, soft mode (auth on for listing but RequireAuth off), or for
// an unowned backend it always allows.
func (a *AuthConfig) authorizePreview(w http.ResponseWriter, r *http.Request, b *Backend, publicHost string) bool {
	if !a.enabled() || !a.RequireAuth || b.Owner == "" {
		return true
	}
	login := a.sessionLogin(r)
	if login == "" {
		// Send the browser through GitHub, returning to the originating URL.
		ret := "https://" + publicHost + r.URL.RequestURI()
		loginURL := "https://" + a.HubHost + "/auth/github/login?return=" + url.QueryEscape(ret)
		http.Redirect(w, r, loginURL, http.StatusFound)
		return false
	}
	if login == b.Owner {
		return true
	}
	a.auditSink().Log(audit.Event{
		Event: audit.EventAccessDeny, Login: login, IP: clientIP(r),
		Subdomain: b.Subdomain, Owner: b.Owner, Outcome: "deny", Detail: "not owner",
	})
	http.Error(w, "403 — this preview belongs to another user", http.StatusForbidden)
	return false
}
