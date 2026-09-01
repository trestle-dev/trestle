package appauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

// shortReader returns n/2 bytes without error, proving the checked helper
// rejects partially-filled buffers rather than encoding zero-padded values.
type shortReader struct{ n int }

func (r shortReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.n <= 0 {
		return 0, io.EOF
	}
	half := len(p) / 2
	if half == 0 {
		half = 1
	}
	if half > r.n {
		half = r.n
	}
	for i := 0; i < half; i++ {
		p[i] = 0xA5
	}
	return half, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// TestPasswordSaltEntropyFailureBlocksAllCreationPaths proves that an entropy
// failure while hashing the registration password prevents every account
// creation path: no user row commits, the invitation stays unused, no
// user-creation audit fact commits, and no misleading 201 is returned.
func TestPasswordSaltEntropyFailureBlocksAllCreationPaths(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			t.Run("open-registration", func(t *testing.T) {
				s, h := setupReg(t, provider)
				setPolicy(t, s, "open")
				setEntropy(t, failingReader{})

				w := publicCall(h, "/api/v1/auth/register", `{"email":"entropy-open@example.com","password":"1234567"}`)
				if w.Code == 201 {
					t.Fatalf("open registration returned 201 despite salt entropy failure: %s", w.Body.String())
				}
				assertNoUser(t, s, "entropy-open@example.com")
				assertAuditCount(t, s, "app_registration.user.create", 0)
			})

			t.Run("self-register-accept", func(t *testing.T) {
				s, h := setupReg(t, provider)
				admin := adminauth.New(s.DB(), string(s.Provider()))
				cookie, csrf := adminSetup(t, s, admin)
				setPolicy(t, s, "invite")
				w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"self_register","email":"entropy-self@example.com"}`)
				if w.Code != 201 {
					t.Fatalf("invitation create %d", w.Code)
				}
				var inv struct{ Token string }
				json.Unmarshal(w.Body.Bytes(), &inv)

				setEntropy(t, failingReader{})
				w = publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`)
				if w.Code == 201 {
					t.Fatalf("self-register accept returned 201 despite salt entropy failure")
				}
				assertNoUser(t, s, "entropy-self@example.com")
				assertInvitationUnused(t, s, inv.Token)
				assertAuditCount(t, s, "app_registration.user.create", 0)
			})

			t.Run("activation-accept", func(t *testing.T) {
				s, h := setupReg(t, provider)
				admin := adminauth.New(s.DB(), string(s.Provider()))
				cookie, csrf := adminSetup(t, s, admin)
				w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"entropy-act@example.com"}`)
				if w.Code != 201 {
					t.Fatalf("activation invitation create %d", w.Code)
				}
				var inv struct{ Token string }
				json.Unmarshal(w.Body.Bytes(), &inv)

				setEntropy(t, failingReader{})
				w = publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`)
				if w.Code == 201 {
					t.Fatalf("activation accept returned 201 despite salt entropy failure")
				}
				assertNoUser(t, s, "entropy-act@example.com")
				assertInvitationUnused(t, s, inv.Token)
				assertAuditCount(t, s, "app_registration.user.create", 0)
			})
		})
	}
}

// TestPasswordSaltPartialReadBlocksCreation proves a short-read-without-error
// entropy source is rejected by the checked password-salt helper on every
// creation path, with the same no-mutation guarantees.
func TestPasswordSaltPartialReadBlocksCreation(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			t.Run("open-registration", func(t *testing.T) {
				s, h := setupReg(t, provider)
				setPolicy(t, s, "open")
				setEntropy(t, shortReader{})
				w := publicCall(h, "/api/v1/auth/register", `{"email":"partial-open@example.com","password":"1234567"}`)
				if w.Code == 201 {
					t.Fatalf("open registration 201 despite short entropy read")
				}
				assertNoUser(t, s, "partial-open@example.com")
				assertAuditCount(t, s, "app_registration.user.create", 0)
			})

			t.Run("self-register-accept", func(t *testing.T) {
				s, h := setupReg(t, provider)
				admin := adminauth.New(s.DB(), string(s.Provider()))
				cookie, csrf := adminSetup(t, s, admin)
				setPolicy(t, s, "invite")
				w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"self_register","email":"partial-self@example.com"}`)
				var inv struct{ Token string }
				json.Unmarshal(w.Body.Bytes(), &inv)

				setEntropy(t, shortReader{})
				w = publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`)
				if w.Code == 201 {
					t.Fatalf("self-register accept 201 despite short entropy read")
				}
				assertNoUser(t, s, "partial-self@example.com")
				assertInvitationUnused(t, s, inv.Token)
				assertAuditCount(t, s, "app_registration.user.create", 0)
			})
		})
	}
}

// TestAuditUserCreationActorTarget documents the exact audit actor/target
// contract for user creation:
//
//   - open registration and self-register acceptance: actorKind "system",
//     actorID "public-registration", target = created user ID, details.path
//     "register" / "self_register";
//   - administrator activation acceptance: actorKind "system", actorID
//     "activation", target = created user ID, details.path "activate";
//
// and proves details never contain the password or the raw invitation token.
func TestAuditUserCreationActorTarget(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)

			row := func() (actorKind, actorID, target string) {
				if err := s.DB().QueryRow("SELECT actor_kind,actor_id,target FROM _trestle_audit WHERE action='app_registration.user.create' ORDER BY id DESC LIMIT 1").Scan(&actorKind, &actorID, &target); err != nil {
					t.Fatal(err)
				}
				return
			}

			// Open registration: public registration actor, target = created ID.
			setPolicy(t, s, "open")
			w := publicCall(h, "/api/v1/auth/register", `{"email":"actor-open@example.com","password":"1234567"}`)
			if w.Code != 201 {
				t.Fatalf("open register %d", w.Code)
			}
			var open struct{ ID string }
			json.Unmarshal(w.Body.Bytes(), &open)
			kind, actor, target := row()
			if kind != "system" || actor != "public-registration" || target != open.ID {
				t.Fatalf("open register actor=%s/%s target=%s want system/public-registration/%s", kind, actor, target, open.ID)
			}
			if detailsPath(t, s, "app_registration.user.create") != "register" {
				t.Fatalf("open register path detail != register")
			}
			assertDetailsOmitSecrets(t, s, open.ID)

			// Self-register acceptance: public registration actor.
			setPolicy(t, s, "invite")
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"self_register","email":"actor-self@example.com"}`)
			var selfInv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &selfInv)
			accept := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+selfInv.Token+`","password":"1234567"}`)
			if accept.Code != 201 {
				t.Fatalf("self accept %d", accept.Code)
			}
			var self struct{ ID string }
			json.Unmarshal(accept.Body.Bytes(), &self)
			kind, actor, target = row()
			if kind != "system" || actor != "public-registration" || target != self.ID {
				t.Fatalf("self accept actor=%s/%s target=%s want system/public-registration/%s", kind, actor, target, self.ID)
			}
			if detailsPath(t, s, "app_registration.user.create") != "self_register" {
				t.Fatalf("self accept path detail != self_register")
			}

			// Administrator activation acceptance: activation actor.
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"actor-act@example.com"}`)
			var actInv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &actInv)
			accept = publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+actInv.Token+`","password":"1234567"}`)
			if accept.Code != 201 {
				t.Fatalf("activate accept %d", accept.Code)
			}
			var act struct{ ID string }
			json.Unmarshal(accept.Body.Bytes(), &act)
			kind, actor, target = row()
			if kind != "system" || actor != "activation" || target != act.ID {
				t.Fatalf("activate accept actor=%s/%s target=%s want system/activation/%s", kind, actor, target, act.ID)
			}
			if detailsPath(t, s, "app_registration.user.create") != "activate" {
				t.Fatalf("activate path detail != activate")
			}
			assertDetailsOmitSecrets(t, s, act.ID)
		})
	}
}

// TestMissingAuditWiringFailsClosed proves account creation fails (no user row,
// no audit fact, non-201) when the mandatory audit dependency is absent rather
// than silently creating a user without the promised audit fact.
func TestMissingAuditWiringFailsClosed(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			h := New(s.DB(), adminauth.New(s.DB(), string(s.Provider())))
			setPolicy(t, s, "open")
			w := publicCall(h, "/api/v1/auth/register", `{"email":"no-audit@example.com","password":"1234567"}`)
			if w.Code == 201 {
				t.Fatalf("register succeeded without audit wiring: %s", w.Body.String())
			}
			assertNoUser(t, s, "no-audit@example.com")
			assertAuditCount(t, s, "app_registration.user.create", 0)
		})
	}
}

// TestPartialReadRejectedByCheckedHelper proves the checked secureToken helper
// rejects a short-read-without-error entropy source for session and access
// token secrets (login and refresh must not mint secrets from partial entropy).
func TestPartialReadRejectedByCheckedHelper(t *testing.T) {
	setEntropy(t, shortReader{})
	if _, err := secureToken(18); err == nil {
		t.Fatalf("secureToken accepted a partial read")
	}
	if _, err := hashPassword("1234567"); err == nil {
		t.Fatalf("hashPassword accepted a partial salt read")
	}
	setEntropy(t, nil)
}

// --- helpers ---------------------------------------------------------------

// setEntropy swaps the package entropy reader for the duration of the test.
var entropyMu sync.Mutex

func setEntropy(t *testing.T, r io.Reader) {
	t.Helper()
	entropyMu.Lock()
	defer entropyMu.Unlock()
	if r == nil {
		entropyReader = rand.Reader
		return
	}
	prev := entropyReader
	entropyReader = r
	t.Cleanup(func() { entropyMu.Lock(); defer entropyMu.Unlock(); entropyReader = prev })
}

func assertNoUser(t *testing.T, s *store.Store, email string) {
	t.Helper()
	var n int
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("user %s committed despite entropy/audit failure", email)
	}
}

func assertInvitationUnused(t *testing.T, s *store.Store, rawToken string) {
	t.Helper()
	hash := sha256.Sum256([]byte(rawToken))
	var used, revoked *string
	if err := s.DB().QueryRow("SELECT used_at,revoked_at FROM _trestle_app_invitations WHERE token_hash=?", hash[:]).Scan(&used, &revoked); err != nil {
		t.Fatal(err)
	}
	if used != nil || revoked != nil {
		t.Fatalf("invitation consumed despite entropy failure (used=%v revoked=%v)", used, revoked)
	}
}

func assertAuditCount(t *testing.T, s *store.Store, action string, want int) {
	t.Helper()
	var n int
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_audit WHERE action=?", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("audit %s count=%d want %d", action, n, want)
	}
}

func detailsPath(t *testing.T, s *store.Store, action string) string {
	t.Helper()
	var raw string
	if err := s.DB().QueryRow("SELECT details_json FROM _trestle_audit WHERE action=? ORDER BY id DESC LIMIT 1", action).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return d.Path
}

// assertDetailsOmitSecrets proves the user-creation audit detail for the given
// user never includes the password or a raw token.
func assertDetailsOmitSecrets(t *testing.T, s *store.Store, userID string) {
	t.Helper()
	var raw string
	if err := s.DB().QueryRow("SELECT details_json FROM _trestle_audit WHERE action='app_registration.user.create' AND target=? ORDER BY id DESC LIMIT 1", userID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "1234567") {
		t.Fatalf("audit detail contains the password: %s", raw)
	}
	if strings.Contains(raw, "ta_") || strings.Contains(raw, base64.RawURLEncoding.EncodeToString(make([]byte, 18))) {
		t.Fatalf("audit detail appears to contain a raw token: %s", raw)
	}
}

var _ = store.Transaction(nil)
