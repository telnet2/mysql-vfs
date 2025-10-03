package models

import "time"

// NodeType identifies the type of node stored in the virtual file system.
type NodeType string

const (
	// NodeTypeDirectory represents a directory-like node that can contain children.
	NodeTypeDirectory NodeType = "directory"
	// NodeTypeFile represents a file node that stores inline content.
	NodeTypeFile NodeType = "file"
)

// Node represents a unified file system node. It intentionally avoids foreign keys to
// satisfy the schema requirements defined in the specification.
type Node struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	Path          string     `gorm:"uniqueIndex:idx_nodes_path;size:2048;not null;comment:Absolute path for the node"`
	ParentPath    string     `gorm:"index:idx_nodes_parent;size:2048;comment:Path to the parent node"`
	Type          NodeType   `gorm:"type:enum('file','directory');not null;comment:Node type (file or directory)"`
	TemplatePath  string     `gorm:"size:2048;comment:Path reference to the template file"`
	Metadata      []byte     `gorm:"type:json;comment:Arbitrary metadata stored as JSON"`
	ContentInline []byte     `gorm:"type:longblob;comment:Inline content for small files"`
	ContentURL    string     `gorm:"size:2048;comment:Go Cloud URL when content stored in blob storage"`
	ContentHash   string     `gorm:"size:128;comment:Hash of the content for deduplication and validation"`
	PolicyScript  string     `gorm:"type:longtext;comment:Rego policy script embedded with the node"`
	PolicyHash    string     `gorm:"size:64;comment:Hash of the policy script for caching"`
	Version       uint64     `gorm:"not null;default:1;comment:Optimistic locking version"`
	Deleted       bool       `gorm:"not null;default:false;comment:Soft-delete flag"`
	CreatedAt     time.Time  `gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime"`
	DeletedAt     *time.Time `gorm:"comment:Timestamp when the node was soft deleted"`
}

// NodeEvent defines the event log entry persisted for auditing and webhook replay.
type NodeEvent struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	NodePath    string    `gorm:"index:idx_node_events_path;size:2048;not null;comment:Path of the node associated with the event"`
	EventType   string    `gorm:"size:128;not null;comment:Type of the event emitted"`
	Payload     []byte    `gorm:"type:json;comment:Serialized event payload"`
	User        string    `gorm:"size:256;comment:Identifier of the user triggering the event"`
	Transaction string    `gorm:"size:128;comment:Transaction identifier associated with the event"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

// WebhookRegistration describes webhook subscriptions on a node.
type WebhookRegistration struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	NodePath        string    `gorm:"index:idx_webhooks_node;size:2048;not null;comment:Path of the node where the webhook is attached"`
	EventType       string    `gorm:"size:128;not null;comment:Event type that triggers the webhook"`
	URL             string    `gorm:"size:2048;not null;comment:Target URL of the webhook"`
	CallbackURL     string    `gorm:"size:2048;comment:Callback URL for asynchronous completion"`
	TimeoutSeconds  uint64    `gorm:"not null;default:60;comment:Timeout for webhook completion"`
	RetryPolicy     []byte    `gorm:"type:json;comment:Retry policy configuration"`
	CompensationURL string    `gorm:"size:2048;comment:URL invoked when compensation is required"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

// CronTask stores directory level cron jobs.
type CronTask struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"`
	Directory  string     `gorm:"index:idx_cron_directory;size:2048;not null;comment:Directory path owning the cron task"`
	Expression string     `gorm:"size:128;not null;comment:Cron expression"`
	Payload    []byte     `gorm:"type:json;comment:Serialized task payload"`
	Enabled    bool       `gorm:"not null;default:true;comment:Whether the task is enabled"`
	LastRunAt  *time.Time `gorm:"comment:Timestamp of the last run"`
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime"`
}

// WorkflowState stores workflow metadata derived from templates.
type WorkflowState struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	TemplatePath  string    `gorm:"index:idx_workflow_template;size:2048;not null;comment:Template defining the workflow"`
	StatePath     string    `gorm:"size:2048;not null;comment:Directory representing the workflow state"`
	AllowedMoves  []byte    `gorm:"type:json;comment:List of allowed transitions as JSON"`
	Timeout       uint64    `gorm:"not null;default:0;comment:Timeout in seconds before automatic action"`
	TimeoutAction []byte    `gorm:"type:json;comment:Serialized action executed on timeout"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}
