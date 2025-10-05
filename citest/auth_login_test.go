package citest

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
	vfshandlers "github.com/telnet2/mysql-vfs/services/vfs/handlers"
	"golang.org/x/crypto/bcrypt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/golang-jwt/jwt/v5"
)

var _ = Describe("Auth Login Endpoint E2E", Ordered, func() {
	var (
		ctx         context.Context
		testDB      *fixtures.TestDatabase
		testStorage *fixtures.TestS3
		fileService *domain.FileService
		userLoader  *domain.UserLoader
		groupLoader *domain.GroupLoader
		authHandler *vfshandlers.AuthHandler
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Auth Login test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo := mysql.NewGormFileRepository(testDB.GetDB(), testStorage.Storage)
		dirRepo := mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Ensure root directory exists
		_, err := dirRepo.FindByPath(ctx, "/")
		if err != nil {
			// Create root directory manually
			rootDir := &models.Directory{
				ID:        "root",
				Name:      "",
				Path:      "/",
				ParentID:  nil,
			}
			err = testDB.GetDB().Create(rootDir).Error
			Expect(err).NotTo(HaveOccurred())
		}

		// Create loaders
		userLoader = domain.NewUserLoader(fileRepo, dirRepo, 5*time.Minute)
		groupLoader = domain.NewGroupLoader(fileRepo, dirRepo, 5*time.Minute)

		// Create services
		fileService = domain.NewFileService(testDB.GetDB(), testStorage.Storage)

		// Create auth handler
		authHandler = vfshandlers.NewAuthHandler(userLoader, groupLoader, "test-secret-key", 24*time.Hour)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("when .user and .group files are configured", func() {
		BeforeAll(func() {
			// Create .user file at root
			passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			userConfig := `{
				"users": [
					{
						"user_id": "alice",
						"password_hash": "` + string(passwordHash) + `",
						"groups": ["admin", "project-alpha", "developers"]
					},
					{
						"user_id": "bob",
						"password_hash": "` + string(passwordHash) + `",
						"groups": ["project-alpha", "project-beta", "developers"]
					},
					{
						"user_id": "charlie",
						"password_hash": "` + string(passwordHash) + `",
						"groups": ["project-beta", "developers"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Update .group file at root (it already exists from bootstrap)
			groupConfig := `{
				"groups": [
					{
						"group_id": "admin",
						"members": ["alice"]
					},
					{
						"group_id": "project-alpha",
						"members": ["alice", "bob"]
					},
					{
						"group_id": "project-beta",
						"members": ["bob", "charlie"]
					},
					{
						"group_id": "developers",
						"members": ["alice", "bob", "charlie"]
					}
				]
			}`

			_, err = fileService.UpdateFile(
				ctx,
				"/.group",
				"application/json",
				int64(len(groupConfig)),
				io.NopCloser(strings.NewReader(groupConfig)),
				1, // Expected version
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should authenticate alice with admin role and correct groups", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID:   "alice",
				Password: "password123",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(200))

			var response vfshandlers.LoginResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Verify basic response
			Expect(response.UserID).To(Equal("alice"))
			Expect(response.Groups).To(ContainElements("admin", "project-alpha", "developers"))
			Expect(response.Token).NotTo(BeEmpty())

			// Verify JWT token
			token, err := jwt.ParseWithClaims(response.Token, &vfshandlers.CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte("test-secret-key"), nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(token.Valid).To(BeTrue())

			claims := token.Claims.(*vfshandlers.CustomClaims)
			Expect(claims.UserID).To(Equal("alice"))
			Expect(claims.Groups).To(ContainElements("admin", "project-alpha", "developers"))
		})

		It("should authenticate bob with user role and correct groups", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID:   "bob",
				Password: "password123",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(200))

			var response vfshandlers.LoginResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.UserID).To(Equal("bob"))
			Expect(response.Groups).To(ContainElements("project-alpha", "project-beta", "developers"))
			Expect(response.Token).NotTo(BeEmpty())
		})

		It("should authenticate charlie with correct groups", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID:   "charlie",
				Password: "password123",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(200))

			var response vfshandlers.LoginResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.UserID).To(Equal("charlie"))
			Expect(response.Groups).To(ContainElements("project-beta", "developers"))
			Expect(response.Token).NotTo(BeEmpty())
		})

		It("should reject invalid password", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID:   "alice",
				Password: "wrongpassword",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(401))

			var response vfshandlers.ErrorResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.Error).To(Equal("invalid credentials"))
		})

		It("should reject non-existent user", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID:   "nonexistent",
				Password: "password123",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(401))

			var response vfshandlers.ErrorResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.Error).To(Equal("invalid credentials"))
		})

		It("should reject missing user_id", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				Password: "password123",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(400))

			var response vfshandlers.ErrorResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.Error).To(Equal("user_id is required"))
		})

		It("should reject missing password", func() {
			// Create request
			reqBody := vfshandlers.LoginRequest{
				UserID: "alice",
			}
			bodyBytes, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			c := &app.RequestContext{}
			c.Request.SetBody(bodyBytes)
			c.Request.Header.SetContentTypeBytes([]byte("application/json"))

			// Execute login
			authHandler.Login(ctx, c)

			// Verify response
			Expect(c.Response.StatusCode()).To(Equal(400))

			var response vfshandlers.ErrorResponse
			err = json.Unmarshal(c.Response.Body(), &response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.Error).To(Equal("password is required"))
		})
	})

})
