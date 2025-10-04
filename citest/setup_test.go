package citest

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gavv/httpexpect/v2"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

var (
	testCtx     context.Context
	cancelCtx   context.CancelFunc
	mysqlC      *mysql.MySQLContainer
	localC      *localstack.LocalStackContainer
	metadataEx  *gexec.Session
	contentEx   *gexec.Session
	metadataBin string
	contentBin  string
	configPath  string
	metadataURL string
	contentURL  string
	bucketName  string
)

func init() {
	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(500 * time.Millisecond)
}

var _ = BeforeSuite(func() {
	start := time.Now()
	fmt.Fprintf(GinkgoWriter, "[citest] ⏱  starting integration setup\n")

	var err error
	testCtx, cancelCtx = context.WithCancel(context.Background())

	buildStart := time.Now()
	metadataBin, err = gexec.Build("github.com/telnet2/mysql-vfs/services/metadata")
	Expect(err).NotTo(HaveOccurred())
	contentBin, err = gexec.Build("github.com/telnet2/mysql-vfs/services/content")
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "[citest] 🛠  service binaries built in %s\n", time.Since(buildStart))

	section := time.Now()
	mysqlC, err = mysql.RunContainer(testCtx,
		mysql.WithUsername("vfs"),
		mysql.WithPassword("vfs"),
		mysql.WithDatabase("vfs"),
	)
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "[citest] ✅ MySQL container ready in %s\n", time.Since(section))

	section = time.Now()
	mysqlHost, err := mysqlC.Host(testCtx)
	Expect(err).NotTo(HaveOccurred())
	mysqlPort, err := mysqlC.MappedPort(testCtx, "3306/tcp")
	Expect(err).NotTo(HaveOccurred())

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		"vfs", "vfs", mysqlHost, mysqlPort.Port(), "vfs")
	Eventually(func() error {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Ping()
	}).Should(Succeed())
	fmt.Fprintf(GinkgoWriter, "[citest] ⏱  MySQL ping successful in %s\n", time.Since(section))

	section = time.Now()
	localC, err = localstack.Run(testCtx,
		"localstack/localstack:1.4.0",
		testcontainers.WithEnv(map[string]string{"SERVICES": "s3"}),
	)
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "[citest] ✅ LocalStack ready in %s\n", time.Since(section))

	lsHost, err := localC.Host(testCtx)
	Expect(err).NotTo(HaveOccurred())
	lsPort, err := localC.MappedPort(testCtx, "4566/tcp")
	Expect(err).NotTo(HaveOccurred())
	endpoint := fmt.Sprintf("http://%s:%s", lsHost, lsPort.Port())
	bucketName = fmt.Sprintf("vfs-test-%d", time.Now().UnixNano())
	prepareS3(testCtx, endpoint, bucketName)
	fmt.Fprintf(GinkgoWriter, "[citest] ⏱  S3 bucket initialised in %s\n", time.Since(section))

	metadataPort := mustGetFreePort()
	contentPort := mustGetFreePort()

	bucketURL := buildBucketURL(endpoint, bucketName)
	configPath = writeConfig(metadataPort, contentPort, mysqlHost, mysqlPort.Int(), bucketURL)

	metadataURL = fmt.Sprintf("http://127.0.0.1:%d", metadataPort)
	contentURL = fmt.Sprintf("http://127.0.0.1:%d", contentPort)

	metadataEnv := map[string]string{
		"VFS_CONFIG_PATH":         configPath,
		"AWS_ACCESS_KEY_ID":       "test",
		"AWS_SECRET_ACCESS_KEY":   "test",
		"AWS_REGION":              "us-east-1",
		"AWS_ENDPOINT_URL_S3":     endpoint,
		"AWS_S3_FORCE_PATH_STYLE": "1",
	}
	contentEnv := map[string]string{
		"VFS_CONFIG_PATH":         configPath,
		"AWS_ACCESS_KEY_ID":       "test",
		"AWS_SECRET_ACCESS_KEY":   "test",
		"AWS_REGION":              "us-east-1",
		"AWS_ENDPOINT_URL_S3":     endpoint,
		"AWS_S3_FORCE_PATH_STYLE": "1",
	}

	type serviceConfig struct {
		name      string
		binPath   string
		env       map[string]string
		healthURL string
		target    **gexec.Session
	}

	services := []serviceConfig{
		{
			name:      "metadata",
			binPath:   metadataBin,
			env:       metadataEnv,
			healthURL: fmt.Sprintf("http://127.0.0.1:%d/ping", metadataPort),
			target:    &metadataEx,
		},
		{
			name:      "content",
			binPath:   contentBin,
			env:       contentEnv,
			healthURL: fmt.Sprintf("http://127.0.0.1:%d/ping", contentPort),
			target:    &contentEx,
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(services))
	for _, svc := range services {
		svc := svc
		go func() {
			defer GinkgoRecover()
			defer wg.Done()

			section := time.Now()
			session, err := startService(testCtx, svc.binPath, svc.env)
			Expect(err).NotTo(HaveOccurred())
			*svc.target = session
			waitForHTTP(svc.healthURL)
			fmt.Fprintf(GinkgoWriter, "[citest] ✅ %s service ready in %s\n", svc.name, time.Since(section))
		}()
	}
	wg.Wait()
	fmt.Fprintf(GinkgoWriter, "[citest] ✅ Setup completed in %s\n", time.Since(start))
})

var _ = AfterSuite(func() {
	start := time.Now()
	if cancelCtx != nil {
		cancelCtx()
	}
	svcStart := time.Now()
	stopSession(contentEx)
	stopSession(metadataEx)
	fmt.Fprintf(GinkgoWriter, "[citest] ⏱  Services stopped in %s\n", time.Since(svcStart))
	if localC != nil {
		_ = localC.Terminate(testCtx)
	}
	if mysqlC != nil {
		_ = mysqlC.Terminate(testCtx)
	}
	if configPath != "" {
		_ = os.Remove(configPath)
	}
	gexec.CleanupBuildArtifacts()
	fmt.Fprintf(GinkgoWriter, "[citest] ✅ Teardown completed in %s\n", time.Since(start))
})

func prepareS3(ctx context.Context, endpoint, bucket string) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	Expect(err).NotTo(HaveOccurred())
	cfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{PartitionID: "aws", URL: endpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() error {
		_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
		return err
	}).Should(Succeed())
}

func buildBucketURL(endpoint, bucket string) string {
	parsed, err := url.Parse(endpoint)
	Expect(err).NotTo(HaveOccurred())
	values := url.Values{}
	values.Set("endpoint", fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host))
	values.Set("region", "us-east-1")
	values.Set("s3ForcePathStyle", "true")
	return fmt.Sprintf("s3://%s?%s", bucket, values.Encode())
}

func writeConfig(metadataPort, contentPort int, mysqlHost string, mysqlPort int, bucketURL string) string {
	template := fmt.Sprintf(`services:
  metadata:
    address: "127.0.0.1:%d"
  content:
    address: "127.0.0.1:%d"
  scheduler:
    address: "127.0.0.1:0"
  webhook:
    address: "127.0.0.1:0"
mysql:
  host: "%s"
  port: %d
  user: "vfs"
  password: "vfs"
  database: "vfs"
  params: "charset=utf8mb4&parseTime=True&loc=Local"
blob:
  bucket_url: "%s"
storage:
  inline_json_max_bytes: 10485760
  inline_json_media_types:
    - application/json
    - text/json
webhook:
  callback_secret: ""
  base_url: ""
cron:
  scheduler_interval: 5s
  lock_timeout: 30s
`, metadataPort, contentPort, mysqlHost, mysqlPort, bucketURL)

	dir, err := os.MkdirTemp("", "vfs-config-")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(dir, "config.yaml")
	Expect(os.WriteFile(path, []byte(template), 0o600)).To(Succeed())
	return path
}

func mustGetFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func startService(ctx context.Context, binPath string, env map[string]string) (*gexec.Session, error) {
	cmd := exec.CommandContext(ctx, binPath)
	cmd.Dir = projectRoot()
	cmd.Env = append(os.Environ(), formatEnv(env)...)
	return gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
}

func projectRoot() string {
	wd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	return filepath.Dir(wd)
}

func formatEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func waitForHTTP(url string) {
	Eventually(func() error {
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		return nil
	}).Should(Succeed())
}

func uploadContent(client *httpexpect.Expect, name, mime string, data []byte) *httpexpect.Object {
	encoded := base64.StdEncoding.EncodeToString(data)
	return client.POST("/api/v1/content").
		WithJSON(map[string]any{
			"name":      name,
			"mime_type": mime,
			"data":      encoded,
		}).
		Expect().
		Status(http.StatusCreated).
		JSON().Object()
}

func stopSession(session *gexec.Session) {
	if session == nil {
		return
	}
	pid := 0
	if session.Command != nil && session.Command.Process != nil {
		pid = session.Command.Process.Pid
	}
	start := time.Now()
	fmt.Printf("[citest] 🔻 Terminate PID %d\n", pid)
	session.Terminate()
	if !waitForExit(session, 5*time.Second) {
		fmt.Printf("[citest] ⚠️ PID %d did not exit within 5s, sending SIGKILL\n", pid)
		session.Kill()
		if !waitForExit(session, 5*time.Second) {
			fmt.Printf("[citest] ❌ PID %d still running after SIGKILL timeout\n", pid)
		}
	}
	fmt.Printf("[citest] ✅ PID %d stopped in %s\n", pid, time.Since(start))
}

func waitForExit(session *gexec.Session, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-session.Exited:
		return true
	case <-timer.C:
		return false
	}
}
