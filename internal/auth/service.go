package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	apiKeyPrefix     = "ohk_" // otel-hive key
	apiKeyRawBytes   = 32
	bcryptCost       = 12
)

// Service handles all authentication logic.
type Service struct {
	store              *Store
	jwtSecret          []byte
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
}

// NewService constructs an auth service. jwtSecret must not be empty.
func NewService(store *Store, jwtSecret string, accessExpiry, refreshExpiry time.Duration) *Service {
	return &Service{
		store:              store,
		jwtSecret:          []byte(jwtSecret),
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
	}
}

// IsSetupRequired returns true when no users exist yet.
func (s *Service) IsSetupRequired(ctx context.Context) (bool, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// Setup creates the initial admin user. Fails if any user already exists.
func (s *Service) Setup(ctx context.Context, username, password string) (*TokenPair, error) {
	required, err := s.IsSetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	if !required {
		return nil, ErrSetupDone
	}

	id, err := generateID()
	if err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		ID:           id,
		Username:     username,
		PasswordHash: string(hash),
		Role:         RoleAdmin,
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return s.issueTokenPair(ctx, user)
}

// Login authenticates with username + password and returns a token pair.
func (s *Service) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err == ErrNotFound {
		return nil, ErrInvalidPassword // don't reveal that the user doesn't exist
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	return s.issueTokenPair(ctx, user)
}

// Refresh exchanges a valid refresh token for a new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	hash := hashToken(refreshToken)
	userID, expiresAt, err := s.store.GetRefreshToken(ctx, hash)
	if err == ErrNotFound {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(expiresAt) {
		_ = s.store.DeleteRefreshToken(ctx, hash)
		return nil, ErrTokenExpired
	}

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Rotate: delete old token, issue new pair
	_ = s.store.DeleteRefreshToken(ctx, hash)
	return s.issueTokenPair(ctx, user)
}

// Logout invalidates all refresh tokens for the user who owns the given refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)
	userID, _, err := s.store.GetRefreshToken(ctx, hash)
	if err == ErrNotFound {
		return nil // already logged out
	}
	if err != nil {
		return err
	}
	return s.store.DeleteUserRefreshTokens(ctx, userID)
}

// ValidateAccessToken parses and validates a JWT access token.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ValidateApiKey checks an API key and returns the owning user on success.
func (s *Service) ValidateApiKey(ctx context.Context, rawKey string) (*User, *ApiKey, error) {
	if !strings.HasPrefix(rawKey, apiKeyPrefix) {
		return nil, nil, ErrInvalidToken
	}
	hash := hmacKeyHash(rawKey, s.jwtSecret)
	key, err := s.store.GetApiKeyByHash(ctx, hash)
	if err == ErrNotFound {
		return nil, nil, ErrInvalidToken
	}
	if err != nil {
		return nil, nil, err
	}

	user, err := s.store.GetUserByID(ctx, key.UserID)
	if err != nil {
		return nil, nil, err
	}

	// Fire-and-forget last-used update
	_ = s.store.TouchApiKeyLastUsed(ctx, key.ID)
	return user, key, nil
}

// CreateApiKey generates a new API key for the given user. Returns the key
// struct (without hash) and the plaintext key — shown only once.
func (s *Service) CreateApiKey(ctx context.Context, userID, name string) (*ApiKey, string, error) {
	id, err := generateID()
	if err != nil {
		return nil, "", err
	}

	raw, err := generateRawApiKey()
	if err != nil {
		return nil, "", err
	}

	key := &ApiKey{
		ID:      id,
		UserID:  userID,
		Name:    name,
		KeyHash: hmacKeyHash(raw, s.jwtSecret),
	}
	if err := s.store.CreateApiKey(ctx, key); err != nil {
		return nil, "", err
	}

	return key, raw, nil
}

// ListApiKeys returns all API keys for a user (without hashes).
func (s *Service) ListApiKeys(ctx context.Context, userID string) ([]*ApiKey, error) {
	return s.store.ListApiKeysByUser(ctx, userID)
}

// RevokeApiKey deletes an API key owned by the given user.
func (s *Service) RevokeApiKey(ctx context.Context, keyID, userID string) error {
	return s.store.DeleteApiKey(ctx, keyID, userID)
}

// --- internal helpers ---

func (s *Service) issueTokenPair(ctx context.Context, user *User) (*TokenPair, error) {
	now := time.Now()

	// Access token
	accessClaims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenExpiry)),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	// Refresh token — opaque random bytes, stored as HMAC hash
	rawRefresh, err := generateRawRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshHash := hashToken(rawRefresh)
	expiresAt := now.Add(s.refreshTokenExpiry)
	if err := s.store.StoreRefreshToken(ctx, user.ID, refreshHash, expiresAt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(s.accessTokenExpiry.Seconds()),
	}, nil
}

func generateRawApiKey() (string, error) {
	b := make([]byte, apiKeyRawBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return apiKeyPrefix + hex.EncodeToString(b), nil
}

func generateRawRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken produces a SHA-256 hex of a token — used for refresh token storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// hmacKeyHash produces an HMAC-SHA256 hex of an API key — uses the JWT secret
// as the HMAC key so the hash is useless without the secret.
func hmacKeyHash(key string, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}
