package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

var ErrInvalidRequest = errors.New("invalid identity request")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrHashBusy = errors.New("credential hashing capacity is busy")
var ErrForbidden = errors.New("forbidden")

const (
	argonMemory      = 32 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonSaltLength  = 16
	argonKeyLength   = 32
	accessTokenLife  = 15 * time.Minute
	refreshTokenLife = 30 * 24 * time.Hour
)

type Service struct {
	db       *sql.DB
	secret   []byte
	issuer   string
	audience string
	hashSlot chan struct{}
}

func NewService(db *sql.DB, secret []byte, issuer, audience string) (*Service, error) {
	if len(secret) < 32 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate ephemeral JWT key: %w", err)
		}
	}
	return &Service{db: db, secret: append([]byte(nil), secret...), issuer: issuer, audience: audience, hashSlot: make(chan struct{}, 1)}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (Tokens, error) {
	email, err := normalizeEmail(email)
	if err != nil || len(password) < 12 || len(password) > 128 {
		return Tokens{}, ErrInvalidRequest
	}
	passwordHash, err := s.hashPassword(password)
	if err != nil {
		return Tokens{}, err
	}
	userID, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tokens{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,email_normalized,password_hash,role,created_at) VALUES(?,?,?,'user',?)", userID, email, passwordHash, now.Format(time.RFC3339Nano)); err != nil {
		if isUniqueError(err) {
			return Tokens{}, ErrInvalidCredentials
		}
		return Tokens{}, err
	}
	user := User{ID: userID, Email: email, Role: "user", CreatedAt: now}
	tokens, err := s.createSessionTx(ctx, tx, user, now)
	if err != nil {
		return Tokens{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

// CreateAdmin bootstraps a privileged account for the local operator CLI only.
func (s *Service) CreateAdmin(ctx context.Context, email, password string) error {
	email, err := normalizeEmail(email)
	if err != nil || len(password) < 12 || len(password) > 128 {
		return ErrInvalidRequest
	}
	encoded, err := s.hashPassword(password)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO users(id,email_normalized,password_hash,role,created_at) VALUES(?,?,?,'admin',?)", id, email, encoded, time.Now().UTC().Format(time.RFC3339Nano))
	if isUniqueError(err) {
		return ErrInvalidCredentials
	}
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (Tokens, error) {
	email, err := normalizeEmail(email)
	if err != nil || len(password) > 128 {
		return Tokens{}, ErrInvalidCredentials
	}
	var user User
	var passwordHash string
	var createdAt string
	var disabled sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT id,email_normalized,password_hash,role,created_at,disabled_at FROM users WHERE email_normalized=?", email).Scan(&user.ID, &user.Email, &passwordHash, &user.Role, &createdAt, &disabled)
	if err != nil || disabled.Valid {
		return Tokens{}, ErrInvalidCredentials
	}
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Tokens{}, ErrInvalidCredentials
	}
	if err := s.verifyPassword(passwordHash, password); err != nil {
		return Tokens{}, ErrInvalidCredentials
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return s.createSession(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	if len(refreshToken) < 40 || len(refreshToken) > 256 {
		return Tokens{}, ErrInvalidCredentials
	}
	hash := tokenHash(refreshToken)
	var user User
	var sessionID string
	var createdAt, expires string
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.email_normalized,u.role,u.created_at,a.id,a.expires_at,a.revoked_at FROM auth_sessions a JOIN users u ON u.id=a.user_id WHERE a.refresh_token_hash=?`, hash).Scan(&user.ID, &user.Email, &user.Role, &createdAt, &sessionID, &expires, &revoked)
	if err != nil || revoked.Valid || !timestampAfterNow(expires) {
		return Tokens{}, ErrInvalidCredentials
	}
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Tokens{}, ErrInvalidCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tokens{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	updated, err := tx.ExecContext(ctx, "UPDATE auth_sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL", now.Format(time.RFC3339Nano), sessionID)
	if err != nil {
		return Tokens{}, err
	}
	if count, _ := updated.RowsAffected(); count != 1 {
		return Tokens{}, ErrInvalidCredentials
	}
	newSessionID, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	newRefresh, err := randomToken(32)
	if err != nil {
		return Tokens{}, err
	}
	newExpires := now.Add(refreshTokenLife)
	if _, err := tx.ExecContext(ctx, "INSERT INTO auth_sessions(id,user_id,refresh_token_hash,expires_at,created_at) VALUES(?,?,?,?,?)", newSessionID, user.ID, tokenHash(newRefresh), newExpires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Tokens{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tokens{}, err
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return s.tokens(user, newSessionID, newRefresh, now), nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidCredentials
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(s.audience), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.ID == "" {
		return Principal{}, ErrInvalidCredentials
	}
	var role string
	var revoked sql.NullString
	var expires string
	if err := s.db.QueryRowContext(ctx, `SELECT u.role,a.revoked_at,a.expires_at FROM auth_sessions a JOIN users u ON u.id=a.user_id WHERE a.id=? AND a.user_id=? AND u.disabled_at IS NULL`, claims.ID, claims.Subject).Scan(&role, &revoked, &expires); err != nil || revoked.Valid || !timestampAfterNow(expires) {
		return Principal{}, ErrInvalidCredentials
	}
	return Principal{UserID: claims.Subject, Role: role, SessionID: claims.ID}, nil
}

func (s *Service) Logout(ctx context.Context, principal Principal) error {
	_, err := s.db.ExecContext(ctx, "UPDATE auth_sessions SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), principal.SessionID, principal.UserID)
	return err
}

func (s *Service) User(ctx context.Context, userID string) (User, error) {
	var user User
	var createdAt string
	err := s.db.QueryRowContext(ctx, "SELECT id,email_normalized,role,created_at FROM users WHERE id=? AND disabled_at IS NULL", userID).Scan(&user.ID, &user.Email, &user.Role, &createdAt)
	if err == nil {
		user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	}
	return user, err
}

func (s *Service) ProvisionPartnerKey(ctx context.Context, groupName string, scopes []string) (PartnerKey, string, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || len(groupName) > 120 || len(scopes) == 0 {
		return PartnerKey{}, "", ErrInvalidRequest
	}
	allowed := map[string]bool{"catalog:read": true, "gyms:read": true, "calories:estimate": true, "planner:preview": true}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if !allowed[scope] || seen[scope] {
			return PartnerKey{}, "", ErrInvalidRequest
		}
		seen[scope] = true
	}
	raw, err := randomToken(32)
	if err != nil {
		return PartnerKey{}, "", err
	}
	prefix := "amb_" + raw[:8]
	apiKey := prefix + "." + raw
	keyID, err := randomID()
	if err != nil {
		return PartnerKey{}, "", err
	}
	scopeJSON, _ := json.Marshal(scopes)
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO partner_api_keys(id,key_prefix,group_name,key_hash,scopes_json,rate_tier,created_at) VALUES(?,?,?,?,?,'standard',?)", keyID, prefix, groupName, tokenHash(apiKey), string(scopeJSON), now.Format(time.RFC3339Nano)); err != nil {
		return PartnerKey{}, "", err
	}
	return PartnerKey{ID: keyID, GroupName: groupName, Scopes: scopes, RateTier: "standard", CreatedAt: now}, apiKey, nil
}

func (s *Service) AuthenticatePartner(ctx context.Context, apiKey string) (PartnerKey, error) {
	parts := strings.Split(apiKey, ".")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "amb_") || len(apiKey) > 256 {
		return PartnerKey{}, ErrInvalidCredentials
	}
	var key PartnerKey
	var hash, scopeJSON string
	var createdAt string
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT id,group_name,key_hash,scopes_json,rate_tier,created_at,revoked_at FROM partner_api_keys WHERE key_prefix=?", parts[0]).Scan(&key.ID, &key.GroupName, &hash, &scopeJSON, &key.RateTier, &createdAt, &revoked)
	if err != nil || revoked.Valid || subtle.ConstantTimeCompare([]byte(hash), []byte(tokenHash(apiKey))) != 1 {
		return PartnerKey{}, ErrInvalidCredentials
	}
	if err := json.Unmarshal([]byte(scopeJSON), &key.Scopes); err != nil {
		return PartnerKey{}, ErrInvalidCredentials
	}
	key.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return PartnerKey{}, ErrInvalidCredentials
	}
	key.CreatedAt = key.CreatedAt.UTC()
	return key, nil
}

func (s *Service) RevokePartnerKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE partner_api_keys SET revoked_at=? WHERE id=? AND revoked_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Service) RotatePartnerKey(ctx context.Context, id string) (PartnerKey, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PartnerKey{}, "", err
	}
	defer tx.Rollback()
	var group, scopesJSON string
	if err = tx.QueryRowContext(ctx, "SELECT group_name,scopes_json FROM partner_api_keys WHERE id=? AND revoked_at IS NULL", id).Scan(&group, &scopesJSON); err != nil {
		return PartnerKey{}, "", ErrInvalidCredentials
	}
	var scopes []string
	if json.Unmarshal([]byte(scopesJSON), &scopes) != nil {
		return PartnerKey{}, "", ErrInvalidCredentials
	}
	raw, err := randomToken(32)
	if err != nil {
		return PartnerKey{}, "", err
	}
	prefix := "amb_" + raw[:8]
	keyValue := prefix + "." + raw
	newID, err := randomID()
	if err != nil {
		return PartnerKey{}, "", err
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, "INSERT INTO partner_api_keys(id,key_prefix,group_name,key_hash,scopes_json,rate_tier,created_at) VALUES(?,?,?,?,?,'standard',?)", newID, prefix, group, tokenHash(keyValue), scopesJSON, now.Format(time.RFC3339Nano)); err != nil {
		return PartnerKey{}, "", err
	}
	result, err := tx.ExecContext(ctx, "UPDATE partner_api_keys SET revoked_at=? WHERE id=? AND revoked_at IS NULL", now.Format(time.RFC3339Nano), id)
	if err != nil {
		return PartnerKey{}, "", err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return PartnerKey{}, "", ErrInvalidCredentials
	}
	if err = tx.Commit(); err != nil {
		return PartnerKey{}, "", err
	}
	return PartnerKey{ID: newID, GroupName: group, Scopes: scopes, RateTier: "standard", CreatedAt: now}, keyValue, nil
}

func (s *Service) ListPartnerKeys(ctx context.Context) ([]PartnerKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,group_name,scopes_json,rate_tier,created_at,revoked_at FROM partner_api_keys ORDER BY created_at DESC LIMIT 500")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]PartnerKey, 0)
	for rows.Next() {
		var key PartnerKey
		var scopes, created string
		var revoked sql.NullString
		if err = rows.Scan(&key.ID, &key.GroupName, &scopes, &key.RateTier, &created, &revoked); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(scopes), &key.Scopes); err != nil {
			return nil, err
		}
		key.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		if revoked.Valid {
			key.RevokedAt = &revoked.String
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Service) hashPassword(password string) (string, error) {
	select {
	case s.hashSlot <- struct{}{}:
		defer func() { <-s.hashSlot }()
	default:
		return "", ErrHashBusy
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonIterations, argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func (s *Service) verifyPassword(encoded, password string) error {
	select {
	case s.hashSlot <- struct{}{}:
		defer func() { <-s.hashSlot }()
	default:
		return ErrHashBusy
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return ErrInvalidCredentials
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 8*1024 || memory > 64*1024 || iterations < 1 || iterations > 4 || parallelism < 1 || parallelism > 4 {
		return ErrInvalidCredentials
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return ErrInvalidCredentials
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) != 32 {
		return ErrInvalidCredentials
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *Service) createSession(ctx context.Context, user User) (Tokens, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tokens{}, err
	}
	defer tx.Rollback()
	tokens, err := s.createSessionTx(ctx, tx, user, time.Now().UTC())
	if err != nil {
		return Tokens{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

func (s *Service) createSessionTx(ctx context.Context, tx *sql.Tx, user User, now time.Time) (Tokens, error) {
	sessionID, err := randomID()
	if err != nil {
		return Tokens{}, err
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return Tokens{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO auth_sessions(id,user_id,refresh_token_hash,expires_at,created_at) VALUES(?,?,?,?,?)", sessionID, user.ID, tokenHash(refreshToken), now.Add(refreshTokenLife).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Tokens{}, err
	}
	return s.tokens(user, sessionID, refreshToken, now), nil
}

func (s *Service) tokens(user User, sessionID, refreshToken string, now time.Time) Tokens {
	expires := now.Add(accessTokenLife)
	claims := jwt.RegisteredClaims{Issuer: s.issuer, Subject: user.ID, Audience: jwt.ClaimStrings{s.audience}, ExpiresAt: jwt.NewNumericDate(expires), NotBefore: jwt.NewNumericDate(now.Add(-time.Second)), IssuedAt: jwt.NewNumericDate(now), ID: sessionID}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	access, _ := token.SignedString(s.secret)
	return Tokens{User: user, AccessToken: access, RefreshToken: refreshToken, TokenType: "Bearer", AccessTokenExpiresAt: expires, RefreshTokenExpiresAt: now.Add(refreshTokenLife)}
}

func normalizeEmail(email string) (string, error) {
	if len(email) == 0 || len(email) > 254 || strings.ContainsAny(email, "\r\n\t ") {
		return "", ErrInvalidRequest
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsed.Address, email) {
		return "", ErrInvalidRequest
	}
	return strings.ToLower(email), nil
}

func randomToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func timestampAfterNow(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.After(time.Now().UTC())
}

func isUniqueError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
