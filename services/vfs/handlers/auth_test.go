package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

// MockUserLoader mocks the UserLoader
type MockUserLoader struct {
	mock.Mock
}

func (m *MockUserLoader) LoadUser(ctx context.Context, dirPath, userID string) (*domain.UserCredential, error) {
	args := m.Called(ctx, dirPath, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UserCredential), args.Error(1)
}

func (m *MockUserLoader) ValidatePassword(user *domain.UserCredential, password string) error {
	args := m.Called(user, password)
	return args.Error(0)
}

// MockGroupLoader mocks the GroupLoader
type MockGroupLoader struct {
	mock.Mock
}

func (m *MockGroupLoader) GetUserGroups(ctx context.Context, userID string) ([]string, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]string), args.Error(1)
}

func TestAuthHandler_Login_Success(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	// Create bcrypt hash for "password123"
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &domain.UserCredential{
		UserID:       "alice",
		PasswordHash: string(passwordHash),
		Groups:       []string{"admin", "user"},
	}

	mockUserLoader.On("LoadUser", mock.Anything, "/", "alice").Return(user, nil)
	mockUserLoader.On("ValidatePassword", user, "password123").Return(nil)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID:   "alice",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 200, c.Response.StatusCode())

	var response LoginResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "alice", response.UserID)
	assert.Equal(t, user.Groups, response.Groups)
	assert.NotEmpty(t, response.Token)

	// Verify JWT token
	token, err := jwt.ParseWithClaims(response.Token, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims := token.Claims.(*CustomClaims)
	assert.Equal(t, "alice", claims.UserID)
	assert.Equal(t, user.Groups, claims.Groups)

	mockUserLoader.AssertExpectations(t)
}

func TestAuthHandler_Login_InvalidPassword(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &domain.UserCredential{
		UserID:       "alice",
		PasswordHash: string(passwordHash),
		Groups:       []string{"admin"},
	}

	mockUserLoader.On("LoadUser", mock.Anything, "/", "alice").Return(user, nil)
	mockUserLoader.On("ValidatePassword", user, "wrongpassword").Return(fmt.Errorf("invalid password"))

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID:   "alice",
		Password: "wrongpassword",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 401, c.Response.StatusCode())

	var response ErrorResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "invalid credentials", response.Error)

	mockUserLoader.AssertExpectations(t)
}

func TestAuthHandler_Login_UserNotFound(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	mockUserLoader.On("LoadUser", mock.Anything, "/", "nonexistent").Return((*domain.UserCredential)(nil), fmt.Errorf("user not found"))

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID:   "nonexistent",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 401, c.Response.StatusCode())

	var response ErrorResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "invalid credentials", response.Error)

	mockUserLoader.AssertExpectations(t)
}

func TestAuthHandler_Login_MissingUserID(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 400, c.Response.StatusCode())

	var response ErrorResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "user_id is required", response.Error)
}

func TestAuthHandler_Login_MissingPassword(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID: "alice",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 400, c.Response.StatusCode())

	var response ErrorResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "password is required", response.Error)
}

func TestAuthHandler_Login_InvalidJSON(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody([]byte("invalid json"))
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 400, c.Response.StatusCode())

	var response ErrorResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "invalid request body", response.Error)
}

func TestAuthHandler_Login_NoGroupsFile(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &domain.UserCredential{
		UserID:       "alice",
		PasswordHash: string(passwordHash),
		Groups:       []string{},
	}

	// User has no groups in .user file
	mockUserLoader.On("LoadUser", mock.Anything, "/", "alice").Return(user, nil)
	mockUserLoader.On("ValidatePassword", user, "password123").Return(nil)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 24*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID:   "alice",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert - should succeed with empty groups
	assert.Equal(t, 200, c.Response.StatusCode())

	var response LoginResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "alice", response.UserID)
	assert.Empty(t, response.Groups)
	assert.NotEmpty(t, response.Token)

	mockUserLoader.AssertExpectations(t)
}

func TestAuthHandler_Login_MultipleGroups(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &domain.UserCredential{
		UserID:       "bob",
		PasswordHash: string(passwordHash),
		Groups:       []string{"project-alpha", "project-beta", "developers"},
	}

	mockUserLoader.On("LoadUser", mock.Anything, "/", "bob").Return(user, nil)
	mockUserLoader.On("ValidatePassword", user, "password123").Return(nil)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 1*time.Hour)

	// Create request
	reqBody := LoginRequest{
		UserID:   "bob",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	ctx := context.Background()
	c := &app.RequestContext{}
	c.Request.SetBody(bodyBytes)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))

	// Execute
	handler.Login(ctx, c)

	// Assert
	assert.Equal(t, 200, c.Response.StatusCode())

	var response LoginResponse
	err := json.Unmarshal(c.Response.Body(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "bob", response.UserID)
	assert.ElementsMatch(t, user.Groups, response.Groups)
	assert.NotEmpty(t, response.Token)

	mockUserLoader.AssertExpectations(t)
}

func TestAuthHandler_GenerateToken_Expiration(t *testing.T) {
	// Setup
	mockUserLoader := new(MockUserLoader)
	mockGroupLoader := new(MockGroupLoader)

	handler := NewAuthHandler(mockUserLoader, mockGroupLoader, "test-secret", 2*time.Second)

	// Generate token
	token, err := handler.generateToken("alice", []string{"admin"})
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse and verify token is valid now
	parsedToken, err := jwt.ParseWithClaims(token, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	assert.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Token should be expired now
	parsedToken, err = jwt.ParseWithClaims(token, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	assert.Error(t, err)
	assert.False(t, parsedToken.Valid)
}
