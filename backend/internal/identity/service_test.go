package identity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func newIdentityService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db, []byte("01234567890123456789012345678901"), "ambatumove", "ambatumove-mobile")
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestRegisterLoginRefreshAndLogoutRevokeSessions(t *testing.T) {
	service := newIdentityService(t)
	first, err := service.Register(context.Background(), "Athlete@example.com", "a secure demo password")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := service.Authenticate(context.Background(), first.AccessToken)
	if err != nil || principal.UserID != first.User.ID {
		t.Fatalf("Authenticate() principal=%+v err=%v", principal, err)
	}
	if _, err := service.Authenticate(context.Background(), first.RefreshToken); err == nil {
		t.Fatal("refresh token was accepted as access token")
	}
	second, err := service.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if _, err := service.Refresh(context.Background(), first.RefreshToken); err == nil {
		t.Fatal("rotated refresh token remained usable")
	}
	if _, err := service.Authenticate(context.Background(), first.AccessToken); err == nil {
		t.Fatal("access token from revoked session remained usable")
	}
	principal, err = service.Authenticate(context.Background(), second.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), second.AccessToken); err == nil {
		t.Fatal("logged-out access token remained usable")
	}
	loggedIn, err := service.Login(context.Background(), "athlete@example.com", "a secure demo password")
	if err != nil || loggedIn.User.ID != first.User.ID {
		t.Fatalf("Login() user=%+v err=%v", loggedIn.User, err)
	}
}

func TestRegisterDoesNotStorePasswordAndRejectsDuplicateAccount(t *testing.T) {
	service := newIdentityService(t)
	first, err := service.Register(context.Background(), "private@example.com", "long enough demo password")
	if err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := service.db.QueryRowContext(context.Background(), "SELECT password_hash FROM users WHERE id=?", first.User.ID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash == "long enough demo password" || passwordHash[:8] != "argon2id" {
		t.Fatalf("password is not stored using versioned Argon2id format: %s", passwordHash)
	}
	if _, err := service.Register(context.Background(), "PRIVATE@example.com", "long enough demo password"); err != ErrInvalidCredentials {
		t.Fatalf("duplicate registration error = %v", err)
	}
}

func TestPartnerKeyIsHashedScopedAndRevocable(t *testing.T) {
	service := newIdentityService(t)
	key, secret, err := service.ProvisionPartnerKey(context.Background(), "partner-demo", []string{"catalog:read", "planner:preview"})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := service.db.QueryRowContext(context.Background(), "SELECT key_hash FROM partner_api_keys WHERE id=?", key.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == secret {
		t.Fatal("API key was stored in plaintext")
	}
	resolved, err := service.AuthenticatePartner(context.Background(), secret)
	if err != nil || resolved.ID != key.ID || len(resolved.Scopes) != 2 {
		t.Fatalf("AuthenticatePartner() key=%+v err=%v", resolved, err)
	}
	if err := service.RevokePartnerKey(context.Background(), key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticatePartner(context.Background(), secret); err == nil {
		t.Fatal("revoked API key remained usable")
	}
}

func TestRegisterRejectsMalformedEmailAndShortPassword(t *testing.T) {
	service := newIdentityService(t)
	if _, err := service.Register(context.Background(), "not-an-email", "short"); err != ErrInvalidRequest {
		t.Fatalf("Register() error = %v", err)
	}
}
