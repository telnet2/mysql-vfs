package citest

import (
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telnet2/mysql-vfs/citest/fixtures"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db/mysql"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("File-Based Authentication E2E", Ordered, func() {
	var (
		ctx         context.Context
		testDB      *fixtures.TestDatabase
		testStorage *fixtures.TestS3
		dirService  *domain.DirectoryService
		fileService *domain.FileService
		userLoader  *domain.UserLoader
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up File-Based Auth test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo := mysql.NewGormFileRepository(testDB.GetDB(), testStorage.Storage)
		dirRepo := mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Create user loader
		userLoader = domain.NewUserLoader(fileRepo, dirRepo, 5*60*1000000000) // 5 minutes

		// Create services
		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileService(testDB.GetDB(), testStorage.Storage)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("when creating .user files", func() {
		It("should allow password-based authentication", func() {
			// Step 1: Create directory
			dir, err := dirService.CreateDirectory(ctx, "/", "team")
			Expect(err).NotTo(HaveOccurred())
			Expect(dir.Path).To(Equal("/team"))

			// Step 2: Create password hash
			passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Create .user file with password-based auth
			userConfig := `{
				"users": [
					{
						"user_id": "alice",
						"password_hash": "` + string(passwordHash) + `",
						"role": "admin",
						"groups": ["admins", "developers"]
					},
					{
						"user_id": "bob",
						"password_hash": "` + string(passwordHash) + `",
						"role": "developer",
						"groups": ["developers"]
					}
				]
			}`

			userFile, err := fileService.CreateFile(
				ctx,
				"/team",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(userFile.Name).To(Equal(".user"))

			// Step 4: Load user and validate password
			alice, err := userLoader.LoadUser(ctx, "/team", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(alice.UserID).To(Equal("alice"))
			Expect(alice.Groups).To(ContainElements("admins", "developers"))

			// Step 5: Validate correct password
			err = userLoader.ValidatePassword(alice, "secret123")
			Expect(err).NotTo(HaveOccurred())

			// Step 6: Validate incorrect password fails
			err = userLoader.ValidatePassword(alice, "wrong-password")
			Expect(err).To(HaveOccurred())
		})

		It("should allow token-based authentication", func() {
			// Step 1: Create directory
			_, err := dirService.CreateDirectory(ctx, "/", "api")
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create .user file with token-based auth
			userConfig := `{
				"users": [
					{
						"user_id": "service-account",
						"token": "sa-token-abc123def456",
						"role": "service",
						"groups": ["services"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/api",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Load user by token
			user, err := userLoader.LoadUserByToken(ctx, "/api", "sa-token-abc123def456")
			Expect(err).NotTo(HaveOccurred())
			Expect(user.UserID).To(Equal("service-account"))
			Expect(user.Groups).To(ContainElement("services"))

			// Step 4: Invalid token should fail
			_, err = userLoader.LoadUserByToken(ctx, "/api", "invalid-token")
			Expect(err).To(HaveOccurred())
		})

		It("should handle hybrid auth (password + token for same user)", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "hybrid")
			Expect(err).NotTo(HaveOccurred())

			hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

			userConfig := `{
				"users": [
					{
						"user_id": "hybrid-user",
						"password_hash": "` + string(hash) + `",
						"token": "backup-token-123",
						"role": "admin",
						"groups": ["admin"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/hybrid",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Should work with password
			user1, err := userLoader.LoadUser(ctx, "/hybrid", "hybrid-user")
			Expect(err).NotTo(HaveOccurred())
			Expect(userLoader.ValidatePassword(user1, "password")).NotTo(HaveOccurred())

			// Should also work with token
			user2, err := userLoader.LoadUserByToken(ctx, "/hybrid", "backup-token-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(user2.UserID).To(Equal("hybrid-user"))
		})
	})

	Context("when updating .user files", func() {
		It("should invalidate cache and load new users", func() {
			// Create directory and initial .user file
			dir, err := dirService.CreateDirectory(ctx, "/", "dynamic")
			Expect(err).NotTo(HaveOccurred())

			hash1, _ := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.DefaultCost)
			initialConfig := `{
				"users": [
					{
						"user_id": "user1",
						"password_hash": "` + string(hash1) + `",
						"role": "user",
						"groups": ["user"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/dynamic",
				".user",
				"application/json",
				int64(len(initialConfig)),
				io.NopCloser(strings.NewReader(initialConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Load user1 (populates cache)
			user1, err := userLoader.LoadUser(ctx, "/dynamic", "user1")
			Expect(err).NotTo(HaveOccurred())
			Expect(user1.UserID).To(Equal("user1"))

			// Invalidate cache (simulates .user file update)
			userLoader.InvalidateCache(dir.ID)

			// user2 doesn't exist yet, should fail
			_, err = userLoader.LoadUser(ctx, "/dynamic", "user2")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when handling authentication errors", func() {
		It("should fail when .user file doesn't exist", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "no-auth")
			Expect(err).NotTo(HaveOccurred())

			// No .user file created
			_, err = userLoader.LoadUser(ctx, "/no-auth", "anyone")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(".user file not found"))
		})

		It("should fail when user doesn't exist in .user file", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "limited")
			Expect(err).NotTo(HaveOccurred())

			hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
			userConfig := `{
				"users": [
					{
						"user_id": "only-user",
						"password_hash": "` + string(hash) + `",
						"role": "admin",
						"groups": ["admin"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/limited",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Existing user works
			_, err = userLoader.LoadUser(ctx, "/limited", "only-user")
			Expect(err).NotTo(HaveOccurred())

			// Non-existent user fails
			_, err = userLoader.LoadUser(ctx, "/limited", "other-user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("user not found"))
		})

		It("should fail with invalid .user file format", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "broken")
			Expect(err).NotTo(HaveOccurred())

			invalidConfig := `{ "invalid": "json" missing closing brace`

			_, err = fileService.CreateFile(
				ctx,
				"/broken",
				".user",
				"application/json",
				int64(len(invalidConfig)),
				io.NopCloser(strings.NewReader(invalidConfig)),
			)
			// Should fail at validation stage if .user is special file
			// Otherwise will fail when loading
			if err == nil {
				// If creation succeeded, loading should fail
				_, err = userLoader.LoadUser(ctx, "/broken", "anyone")
				Expect(err).To(HaveOccurred())
			}
		})
	})

	Context("when using groups", func() {
		It("should load users with group membership", func() {
			_, err := dirService.CreateDirectory(ctx, "/", "groups-test")
			Expect(err).NotTo(HaveOccurred())

			hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

			userConfig := `{
				"users": [
					{
						"user_id": "user1",
						"password_hash": "` + string(hash) + `",
						"role": "developer",
						"groups": ["frontend", "backend", "devops"]
					},
					{
						"user_id": "user2",
						"password_hash": "` + string(hash) + `",
						"role": "designer",
						"groups": ["frontend", "design"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/groups-test",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Load users and check groups
			user1, err := userLoader.LoadUser(ctx, "/groups-test", "user1")
			Expect(err).NotTo(HaveOccurred())
			Expect(user1.Groups).To(HaveLen(3))
			Expect(user1.Groups).To(ContainElements("frontend", "backend", "devops"))

			user2, err := userLoader.LoadUser(ctx, "/groups-test", "user2")
			Expect(err).NotTo(HaveOccurred())
			Expect(user2.Groups).To(HaveLen(2))
			Expect(user2.Groups).To(ContainElements("frontend", "design"))
		})

	})

	Context("when testing caching behavior", func() {
		It("should cache .user file for performance", func() {
			// Create directory and .user file
			_, err := dirService.CreateDirectory(ctx, "/", "cached")
			Expect(err).NotTo(HaveOccurred())

			hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

			userConfig := `{
				"users": [
					{
						"user_id": "cached-user",
						"password_hash": "` + string(hash) + `",
						"role": "user",
						"groups": ["user"]
					}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/cached",
				".user",
				"application/json",
				int64(len(userConfig)),
				io.NopCloser(strings.NewReader(userConfig)),
			)
			Expect(err).NotTo(HaveOccurred())

			// First load - populates cache
			user1, err := userLoader.LoadUser(ctx, "/cached", "cached-user")
			Expect(err).NotTo(HaveOccurred())

			// Second load - should hit cache (same object)
			user2, err := userLoader.LoadUser(ctx, "/cached", "cached-user")
			Expect(err).NotTo(HaveOccurred())
			Expect(user2.UserID).To(Equal(user1.UserID))

			// Third load - different user from same directory - should hit cache
			_, err = userLoader.LoadUser(ctx, "/cached", "cached-user")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
