package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/golang-jwt/jwt/v5"
	"github.com/telnet2/mysql-vfs/pkg/domain"
)

// UserLoader interface for loading users
type UserLoader interface {
	LoadUser(ctx context.Context, dirPath, userID string) (*domain.UserCredential, error)
	ValidatePassword(user *domain.UserCredential, password string) error
}

// GroupLoader interface for resolving user groups
type GroupLoader interface {
	GetUserGroups(ctx context.Context, userID string) ([]string, error)
}

// AuthHandler handles authentication operations
type AuthHandler struct {
	userLoader  UserLoader
	groupLoader GroupLoader
	jwtSecret   string
	jwtTTL      time.Duration
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(userLoader UserLoader, groupLoader GroupLoader, jwtSecret string, jwtTTL time.Duration) *AuthHandler {
	return &AuthHandler{
		userLoader:  userLoader,
		groupLoader: groupLoader,
		jwtSecret:   jwtSecret,
		jwtTTL:      jwtTTL,
	}
}

// LoginRequest represents the login request body
type LoginRequest struct {
	UserID   string `json:"user_id"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token  string   `json:"token"`
	UserID string   `json:"user_id"`
	Groups []string `json:"groups"`
}

// CustomClaims represents JWT claims with user information
type CustomClaims struct {
	UserID string   `json:"user_id"`
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(ctx context.Context, c *app.RequestContext) {
	var req LoginRequest

	// Bind and validate request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	if req.UserID == "" {
		c.JSON(consts.StatusBadRequest, ErrorResponse{
			Error: "user_id is required",
		})
		return
	}

	if req.Password == "" {
		c.JSON(consts.StatusBadRequest, ErrorResponse{
			Error: "password is required",
		})
		return
	}

	// Load user from /.user file
	user, err := h.userLoader.LoadUser(ctx, "/", req.UserID)
	if err != nil {
		c.JSON(consts.StatusUnauthorized, ErrorResponse{
			Error: "invalid credentials",
		})
		return
	}

	// Validate password
	if err := h.userLoader.ValidatePassword(user, req.Password); err != nil {
		c.JSON(consts.StatusUnauthorized, ErrorResponse{
			Error: "invalid credentials",
		})
		return
	}

	// Generate JWT token with user's groups
	token, err := h.generateToken(user.UserID, user.Groups)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, ErrorResponse{
			Error: "failed to generate token",
		})
		return
	}

	// Return response
	c.JSON(consts.StatusOK, LoginResponse{
		Token:  token,
		UserID: user.UserID,
		Groups: user.Groups,
	})
}

// generateToken creates a JWT token with user information
func (h *AuthHandler) generateToken(userID string, groups []string) (string, error) {
	// Create claims
	claims := CustomClaims{
		UserID: userID,
		Groups: groups,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(h.jwtTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "mysql-vfs",
		},
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token
	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}
