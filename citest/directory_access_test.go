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
)

var _ = Describe("Directory Access Control E2E", Ordered, func() {
	var (
		ctx          context.Context
		testDB       *fixtures.TestDatabase
		testStorage  *fixtures.TestS3
		dirService   *domain.DirectoryService
		fileService  *domain.FileService
		filesLoader  *domain.FilesLoader
		groupLoader  *domain.GroupLoader
		ownerLoader  *domain.OwnerLoader
		fileRepo     *mysql.GormFileRepository
		dirRepo      *mysql.GormDirectoryRepository
	)

	BeforeAll(func() {
		GinkgoWriter.Println("🚀 Setting up Directory Access Control test environment...")
		testDB = fixtures.NewTestDatabase()
		testStorage = fixtures.NewTestS3()
		ctx = context.Background()

		// Create repositories
		fileRepo = mysql.NewGormFileRepository(testDB.GetDB(), testStorage.Storage)
		dirRepo = mysql.NewGormDirectoryRepository(testDB.GetDB())

		// Create loaders
		filesLoader = domain.NewFilesLoader(fileRepo, dirRepo, 5*60*1000000000)  // 5 minutes
		groupLoader = domain.NewGroupLoader(fileRepo, dirRepo, 5*60*1000000000) // 5 minutes
		ownerLoader = domain.NewOwnerLoader(fileRepo, dirRepo, groupLoader, 5*60*1000000000)

		// Create services
		dirService = domain.NewDirectoryService(testDB.GetDB())
		fileService = domain.NewFileServiceWithGroupValidation(testDB.GetDB(), testStorage.Storage, filesLoader, groupLoader)

		// Update .group file with test groups (bootstrap creates default admin/user groups)
		groupConfig := `{
			"groups": [
				{
					"group_id": "admin",
					"members": ["alice", "bob"]
				},
				{
					"group_id": "user",
					"members": ["charlie", "david", "alice"]
				},
				{
					"group_id": "project-alpha",
					"members": ["alice", "eve"]
				},
				{
					"group_id": "project-beta",
					"members": ["bob", "charlie"]
				}
			]
		}`

		_, err := fileService.UpdateFile(
			ctx,
			"/.group",
			"application/json",
			int64(len(groupConfig)),
			io.NopCloser(strings.NewReader(groupConfig)),
			1, // Expected version
		)
		Expect(err).NotTo(HaveOccurred())

		// Invalidate group cache after updating .group file
		rootDir, _ := dirRepo.FindByPath(ctx, "/")
		groupLoader.InvalidateCache(rootDir.ID)

		GinkgoWriter.Println("✅ Test environment ready")
	})

	AfterAll(func() {
		testDB.Cleanup()
		testStorage.Cleanup()
	})

	Context("Group-based directory ownership", func() {
		BeforeEach(func() {
			// Get root directory
			_, err := dirRepo.FindByPath(ctx, "/")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should resolve user groups correctly", func() {
			// Alice is in admin, user, and project-alpha
			aliceGroups, err := groupLoader.GetUserGroups(ctx, "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceGroups).To(HaveLen(3))
			Expect(aliceGroups).To(ContainElements("admin", "user", "project-alpha"))

			// Bob is in admin and project-beta
			bobGroups, err := groupLoader.GetUserGroups(ctx, "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(bobGroups).To(HaveLen(2))
			Expect(bobGroups).To(ContainElements("admin", "project-beta"))

			// Charlie is in user and project-beta
			charlieGroups, err := groupLoader.GetUserGroups(ctx, "charlie")
			Expect(err).NotTo(HaveOccurred())
			Expect(charlieGroups).To(HaveLen(2))
			Expect(charlieGroups).To(ContainElements("user", "project-beta"))

			// Unknown user has no groups
			unknownGroups, err := groupLoader.GetUserGroups(ctx, "unknown")
			Expect(err).NotTo(HaveOccurred())
			Expect(unknownGroups).To(HaveLen(0))

			GinkgoWriter.Println("✅ User group resolution working correctly")
		})

		It("should validate that groups exist when creating .owner files", func() {
			// Step 1: Create project directory
			_, err := dirService.CreateDirectory(ctx, "/", "projects")
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Try to create .owner with non-existent group (should fail)
			invalidOwnerConfig := `{
				"owners": ["non-existent-group"]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/projects",
				".owner",
				"application/json",
				int64(len(invalidOwnerConfig)),
				io.NopCloser(strings.NewReader(invalidOwnerConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist in /.group file"))

			// Step 3: Create .owner with valid group (should succeed)
			validOwnerConfig := `{
				"owners": ["admin", "project-alpha"]
			}`

			ownerFile, err := fileService.CreateFile(
				ctx,
				"/projects",
				".owner",
				"application/json",
				int64(len(validOwnerConfig)),
				io.NopCloser(strings.NewReader(validOwnerConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(ownerFile.Name).To(Equal(".owner"))

			GinkgoWriter.Println("✅ .owner validation working correctly")
		})

		It("should control directory access based on ownership", func() {
			// Get the projects directory
			projectsDir, err := dirRepo.FindByPath(ctx, "/projects")
			Expect(err).NotTo(HaveOccurred())

			// Alice is in admin and project-alpha (owners), should have access
			aliceGroups, _ := groupLoader.GetUserGroups(ctx, "alice")
			aliceHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, aliceGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceHasAccess).To(BeTrue())

			// Bob is in admin (owner), should have access
			bobGroups, _ := groupLoader.GetUserGroups(ctx, "bob")
			bobHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, bobGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(bobHasAccess).To(BeTrue())

			// Eve is in project-alpha (owner), should have access
			eveGroups, err := groupLoader.GetUserGroups(ctx, "eve")
			Expect(err).NotTo(HaveOccurred())
			eveHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, eveGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(eveHasAccess).To(BeTrue())

			// Charlie is only in user and project-beta (not owners), should NOT have access
			charlieGroups, _ := groupLoader.GetUserGroups(ctx, "charlie")
			charlieHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, charlieGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(charlieHasAccess).To(BeFalse())

			// David is only in user (not owner), should NOT have access
			davidGroups, err := groupLoader.GetUserGroups(ctx, "david")
			Expect(err).NotTo(HaveOccurred())
			davidHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, davidGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(davidHasAccess).To(BeFalse())

			GinkgoWriter.Println("✅ Directory access control working correctly")
		})

		It("should inherit .owner from parent directories", func() {
			// Step 1: Create subdirectory under /projects
			alphaDir, err := dirService.CreateDirectory(ctx, "/projects", "alpha")
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Alpha should inherit ownership from /projects
			alphaOwners, err := ownerLoader.GetOwnerGroups(ctx, alphaDir.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(alphaOwners).To(ContainElements("admin", "project-alpha"))

			// Step 3: Alice should have access to alpha (inherited)
			aliceGroups, _ := groupLoader.GetUserGroups(ctx, "alice")
			aliceHasAccess, err := ownerLoader.IsUserOwner(ctx, alphaDir.ID, aliceGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceHasAccess).To(BeTrue())

			// Step 4: Charlie should NOT have access to alpha (inherited restriction)
			charlieGroups, _ := groupLoader.GetUserGroups(ctx, "charlie")
			charlieHasAccess, err := ownerLoader.IsUserOwner(ctx, alphaDir.ID, charlieGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(charlieHasAccess).To(BeFalse())

			GinkgoWriter.Println("✅ Ownership inheritance working correctly")
		})

		It("should override parent ownership with explicit .owner file", func() {
			// Step 1: Create subdirectory beta under /projects
			betaDir, err := dirService.CreateDirectory(ctx, "/projects", "beta")
			Expect(err).NotTo(HaveOccurred())

			// Step 2: Create .owner file for beta with different owners
			betaOwnerConfig := `{
				"owners": ["project-beta"]
			}`

			ownerFile, err := fileService.CreateFile(
				ctx,
				"/projects/beta",
				".owner",
				"application/json",
				int64(len(betaOwnerConfig)),
				io.NopCloser(strings.NewReader(betaOwnerConfig)),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(ownerFile.Name).To(Equal(".owner"))

			// Step 3: Beta should now have project-beta as owner (not inherited from parent)
			betaOwners, err := ownerLoader.GetOwnerGroups(ctx, betaDir.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(betaOwners).To(HaveLen(1))
			Expect(betaOwners).To(ContainElement("project-beta"))

			// Step 4: Bob is in project-beta, should have access
			bobGroups, _ := groupLoader.GetUserGroups(ctx, "bob")
			bobHasAccess, err := ownerLoader.IsUserOwner(ctx, betaDir.ID, bobGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(bobHasAccess).To(BeTrue())

			// Step 5: Alice is NOT in project-beta, should NOT have access (despite being admin)
			aliceGroups, _ := groupLoader.GetUserGroups(ctx, "alice")
			aliceHasAccess, err := ownerLoader.IsUserOwner(ctx, betaDir.ID, aliceGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceHasAccess).To(BeFalse())

			GinkgoWriter.Println("✅ Ownership override working correctly")
		})

		It("should filter visible directories based on ownership", func() {
			// Step 1: Create multiple directories with different ownership
			dir1, _ := dirService.CreateDirectory(ctx, "/", "public")
			dir2, _ := dirService.CreateDirectory(ctx, "/", "admin-only")
			dir3, _ := dirService.CreateDirectory(ctx, "/", "dev-team")

			// Set ownership for admin-only
			adminOnlyOwner := `{"owners": ["admin"]}`
			_, err := fileService.CreateFile(
				ctx,
				"/admin-only",
				".owner",
				"application/json",
				int64(len(adminOnlyOwner)),
				io.NopCloser(strings.NewReader(adminOnlyOwner)),
			)
			Expect(err).NotTo(HaveOccurred())

			// Set ownership for dev-team
			devTeamOwner := `{"owners": ["project-alpha", "project-beta"]}`
			_, err = fileService.CreateFile(
				ctx,
				"/dev-team",
				".owner",
				"application/json",
				int64(len(devTeamOwner)),
				io.NopCloser(strings.NewReader(devTeamOwner)),
			)
			Expect(err).NotTo(HaveOccurred())

			// public has no .owner, so anyone can access

			dirIDs := []string{dir1.ID, dir2.ID, dir3.ID}

			// Step 2: Filter for Alice (in admin and project-alpha)
			aliceGroups, _ := groupLoader.GetUserGroups(ctx, "alice")
			aliceVisible, err := ownerLoader.FilterVisibleDirectories(ctx, dirIDs, aliceGroups)
			Expect(err).NotTo(HaveOccurred())
			// Alice can see all 3: public (no restriction), admin-only (admin), dev-team (project-alpha)
			Expect(aliceVisible).To(HaveLen(3))

			// Step 3: Filter for Charlie (in user and project-beta)
			charlieGroups, _ := groupLoader.GetUserGroups(ctx, "charlie")
			charlieVisible, err := ownerLoader.FilterVisibleDirectories(ctx, dirIDs, charlieGroups)
			Expect(err).NotTo(HaveOccurred())
			// Charlie can see 2: public (no restriction), dev-team (project-beta)
			Expect(charlieVisible).To(HaveLen(2))
			Expect(charlieVisible).NotTo(ContainElement(dir2.ID)) // Cannot see admin-only

			// Step 4: Filter for David (only in user)
			davidGroups, _ := groupLoader.GetUserGroups(ctx, "david")
			davidVisible, err := ownerLoader.FilterVisibleDirectories(ctx, dirIDs, davidGroups)
			Expect(err).NotTo(HaveOccurred())
			// David can see 1: public (no restriction)
			Expect(davidVisible).To(HaveLen(1))
			Expect(davidVisible).To(ContainElement(dir1.ID)) // Only public

			GinkgoWriter.Println("✅ Directory filtering working correctly")
		})

		It("should prevent .group file creation outside root", func() {
			// Create a test directory
			_, err := dirService.CreateDirectory(ctx, "/", "test-dir")
			Expect(err).NotTo(HaveOccurred())

			// Try to create .group in non-root directory (should fail)
			groupConfig := `{
				"groups": [
					{"group_id": "test", "members": ["test"]}
				]
			}`

			_, err = fileService.CreateFile(
				ctx,
				"/test-dir",
				".group",
				"application/json",
				int64(len(groupConfig)),
				io.NopCloser(strings.NewReader(groupConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("can only be created at root directory"))

			GinkgoWriter.Println("✅ .group restriction to root working correctly")
		})

		It("should invalidate cache when .owner file is updated", func() {
			// Step 1: Get initial owner groups
			projectsDir, _ := dirRepo.FindByPath(ctx, "/projects")
			initialOwners, err := ownerLoader.GetOwnerGroups(ctx, projectsDir.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialOwners).To(ContainElements("admin", "project-alpha"))

			// Step 2: Update .owner file
			newOwnerConfig := `{
				"owners": ["user"]
			}`

			// Update .owner file
			ownerFilePath := "/projects/.owner"
			_, err = fileService.UpdateFile(
				ctx,
				ownerFilePath,
				"application/json",
				int64(len(newOwnerConfig)),
				io.NopCloser(strings.NewReader(newOwnerConfig)),
				0, // Expected version (0 = no version check)
			)
			Expect(err).NotTo(HaveOccurred())

			// Invalidate cache manually (in production, this would be done via event handlers)
			ownerLoader.InvalidateCache(projectsDir.ID)

			// Step 3: Cache should be invalidated, new owners should be reflected
			updatedOwners, err := ownerLoader.GetOwnerGroups(ctx, projectsDir.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedOwners).To(HaveLen(1))
			Expect(updatedOwners).To(ContainElement("user"))

			// Step 4: David (in user group) should now have access
			davidGroups, _ := groupLoader.GetUserGroups(ctx, "david")
			davidHasAccess, err := ownerLoader.IsUserOwner(ctx, projectsDir.ID, davidGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(davidHasAccess).To(BeTrue())

			GinkgoWriter.Println("✅ Cache invalidation working correctly")
		})
	})

	Context("Edge cases and validation", func() {
		It("should handle empty owner groups array", func() {
			// Empty owners array should fail validation
			emptyOwnerConfig := `{
				"owners": []
			}`

			_, err := fileService.CreateFile(
				ctx,
				"/",
				".owner",
				"application/json",
				int64(len(emptyOwnerConfig)),
				io.NopCloser(strings.NewReader(emptyOwnerConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one owner"))
		})

		It("should handle duplicate owner groups", func() {
			// Duplicate owners should fail validation
			duplicateOwnerConfig := `{
				"owners": ["admin", "admin"]
			}`

			_, err := fileService.CreateFile(
				ctx,
				"/",
				".owner",
				"application/json",
				int64(len(duplicateOwnerConfig)),
				io.NopCloser(strings.NewReader(duplicateOwnerConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate owner group_id"))
		})

		It("should handle empty group_id in owners", func() {
			// Empty group_id should fail validation
			emptyGroupConfig := `{
				"owners": [""]
			}`

			_, err := fileService.CreateFile(
				ctx,
				"/",
				".owner",
				"application/json",
				int64(len(emptyGroupConfig)),
				io.NopCloser(strings.NewReader(emptyGroupConfig)),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
		})

		It("should allow access when no .owner file exists", func() {
			// Create directory without .owner
			openDir, err := dirService.CreateDirectory(ctx, "/", "open-access")
			Expect(err).NotTo(HaveOccurred())

			// Any user should have access
			anyUserGroups := []string{"some-random-group"}
			hasAccess, err := ownerLoader.IsUserOwner(ctx, openDir.ID, anyUserGroups)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasAccess).To(BeTrue())

			// Even user with no groups should have access
			noGroupsUser := []string{}
			hasAccess, err = ownerLoader.IsUserOwner(ctx, openDir.ID, noGroupsUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasAccess).To(BeTrue())

			GinkgoWriter.Println("✅ Open access (no .owner) working correctly")
		})
	})
})
