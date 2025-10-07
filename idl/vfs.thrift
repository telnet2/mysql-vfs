namespace go vfs

include "api.thrift"

// Common types
struct HealthCheckRequest {}

struct HealthCheckResponse {
    1: required bool healthy
    2: required string status
    3: required string timestamp
}

// Directory operations
struct CreateDirectoryRequest {
    1: required string path (api.vd="len($)>0")
    2: optional map<string, string> metadata
}

struct CreateDirectoryResponse {
    1: required string path
    2: required string created_at
    3: required map<string, string> metadata
}

struct ListDirectoryRequest {
    1: required string path (api.path="path")
    2: optional i32 limit (api.query="limit", api.vd="$>=1 && $<=100")
    3: optional string cursor (api.query="cursor")
    4: optional bool recursive (api.query="recursive", api.vd="$==true || $==false")
}

struct ListDirectoryResponse {
    1: required list<DirectoryEntry> entries
    2: optional string next_cursor
    3: required i32 total_count
}

struct DirectoryEntry {
    1: required string name
    2: required string type (api.vd="$=='file' || $=='directory'")
    3: required string path
    4: required i64 size_bytes
    5: required string modified_at
    6: required string created_at
    7: optional map<string, string> metadata
}

struct DeleteDirectoryRequest {
    1: required string path (api.path="path")
    2: optional bool recursive (api.query="recursive", api.vd="$==true || $==false")
}

struct DeleteDirectoryResponse {
    1: required bool success
    2: required string message
}

// File operations
struct CreateFileRequest {
    1: required string directory_path (api.vd="len($)>0")
    2: required string name (api.vd="len($)>0")
    3: required string content_type (api.vd="len($)>0")
    4: required i64 size_bytes (api.vd="$>0")
    5: required binary content
    6: required string checksum_sha256 (api.vd="len($)==64")
    7: optional map<string, string> metadata
}

struct CreateFileResponse {
    1: required string path
    2: required string content_type
    3: required i64 size_bytes
    4: required string checksum_sha256
    5: required string created_at
    6: required string modified_at
    7: optional map<string, string> metadata
}

struct GetFileRequest {
    1: required string path (api.path="path")
    2: optional i64 offset (api.query="offset", api.vd="$>=0")
    3: optional i64 limit (api.query="limit", api.vd="$>0")
}

struct GetFileResponse {
    1: required binary content
    2: required string content_type
    3: required i64 size_bytes
    4: required string checksum_sha256
    5: required string modified_at
}

struct GetFileMetadataRequest {
    1: required string path (api.path="path")
}

struct GetFileMetadataResponse {
    1: required string path
    2: required string name
    3: required string content_type
    4: required i64 size_bytes
    5: required string checksum_sha256
    6: required string created_at
    7: required string modified_at
    8: required i32 version
    9: optional map<string, string> metadata
}

struct UpdateFileRequest {
    1: required string path (api.path="path")
    2: optional string content_type (api.vd="len($)>0")
    3: optional binary content
    4: optional string checksum_sha256 (api.vd="len($)==64")
    5: optional map<string, string> metadata
}

struct UpdateFileResponse {
    1: required string path
    2: required string content_type
    3: required i64 size_bytes
    4: required string checksum_sha256
    5: required string modified_at
    6: required i32 version
    7: optional map<string, string> metadata
}

struct DeleteFileRequest {
    1: required string path (api.path="path")
}

struct DeleteFileResponse {
    1: required bool success
    2: required string message
}

struct MoveFileRequest {
    1: required string source_path (api.vd="len($)>0")
    2: required string destination_path (api.vd="len($)>0")
    3: optional bool overwrite (api.vd="$==true || $==false")
}

struct MoveFileResponse {
    1: required string new_path
    2: required string old_path
    3: required bool overwritten
}

struct ListFileVersionsRequest {
    1: required string path (api.path="path")
    2: optional i32 limit (api.query="limit", api.vd="$>=1 && $<=100")
    3: optional string cursor (api.query="cursor")
}

struct ListFileVersionsResponse {
    1: required list<FileVersion> versions
    2: optional string next_cursor
    3: required i32 total_count
}

struct FileVersion {
    1: required i32 version
    2: required string checksum_sha256
    3: required i64 size_bytes
    4: required string created_at
    5: required string modified_at
    6: optional map<string, string> metadata
}

// File relations (new)
struct CreateFileRelationRequest {
    1: required string source_path (api.vd="len($)>0")
    2: required string target_path (api.vd="len($)>0")
    3: required string relation_type (api.vd="len($)>0")
    4: optional map<string, string> metadata
}

struct CreateFileRelationResponse {
    1: required string relation_id
    2: required string source_path
    3: required string target_path
    4: required string relation_type
    5: required string created_at
}

struct ListFileRelationsRequest {
    1: required string path (api.path="path")
    2: optional string relation_type (api.query="type")
    3: optional string direction (api.query="direction", api.vd="$=='in' || $=='out' || $=='both'")
}

struct ListFileRelationsResponse {
    1: required list<FileRelation> relations
    2: required i32 total_count
}

struct FileRelation {
    1: required string relation_id
    2: required string source_path
    3: required string target_path
    4: required string relation_type
    5: required string created_at
    6: optional map<string, string> metadata
}

// Workflow operations (new)
struct GetWorkflowInfoRequest {
    1: required string filepath (api.path="filepath")
}

struct GetWorkflowInfoResponse {
    1: required string filepath
    2: required string current_state
    3: required map<string, string> metadata
    4: required string last_transition
    5: required string created_at
    6: required string updated_at
}

struct GetValidTransitionsRequest {
    1: required string filepath (api.path="filepath")
}

struct GetValidTransitionsResponse {
    1: required string filepath
    2: required string current_state
    3: required list<string> valid_transitions
    4: required map<string, string> transition_rules
}

struct TransitionToStateRequest {
    1: required string filepath (api.path="filepath")
    2: required string target_state (api.vd="len($)>0")
    3: optional map<string, string> metadata
    4: optional string reason (api.vd="len($)>0")
}

struct TransitionToStateResponse {
    1: required string filepath
    2: required string previous_state
    3: required string new_state
    4: required string transition_id
    5: required string transitioned_at
}

// Search operations (new)
struct SearchFilesRequest {
    1: optional string query (api.query="q", api.vd="len($)>0")
    2: optional string jsonpath (api.query="jsonpath")
    3: optional string jq_filter (api.query="jq")
    4: optional string path_prefix (api.query="path_prefix")
    5: optional string content_type (api.query="content_type")
    6: optional i32 min_size (api.query="min_size", api.vd="$>=0")
    7: optional i32 max_size (api.query="max_size", api.vd="$>=0")
    8: optional string from_date (api.query="from")
    9: optional string to_date (api.query="to")
    10: optional i32 limit (api.query="limit", api.vd="$>=1 && $<=100")
    11: optional string cursor (api.query="cursor")
}

struct SearchFilesResponse {
    1: required list<SearchResult> results
    2: optional string next_cursor
    3: required i32 total_count
}

struct SearchResult {
    1: required string path
    2: required string name
    3: required string type
    4: required i64 size_bytes
    5: required string content_type
    6: required string modified_at
    7: required string checksum_sha256
    8: optional map<string, string> metadata
    9: optional map<string, string> highlights
}

// Auth operations (new)
struct LoginRequest {
    1: required string username (api.vd="len($)>0")
    2: required string password (api.vd="len($)>0")
    3: optional string client_id (api.vd="len($)>0")
}

struct LoginResponse {
    1: required string access_token
    2: required string token_type
    3: required i32 expires_in
    4: required string refresh_token
    5: required string scope
}

// Main service definition
service VFSService {
    // Health check
    HealthCheckResponse health(1: HealthCheckRequest req) (api.get="/health")
    
    // Directory operations
    CreateDirectoryResponse createDirectory(1: CreateDirectoryRequest req) (api.post="/api/v1/directories")
    ListDirectoryResponse listDirectory(1: ListDirectoryRequest req) (api.get="/api/v1/directories/:path")
    DeleteDirectoryResponse deleteDirectory(1: DeleteDirectoryRequest req) (api.delete="/api/v1/directories/:path")
    
    // File operations
    CreateFileResponse createFile(1: CreateFileRequest req) (api.post="/api/v1/files")
    GetFileResponse getFile(1: GetFileRequest req) (api.get="/api/v1/files/:path")
    GetFileMetadataResponse getFileMetadata(1: GetFileMetadataRequest req) (api.get="/api/v1/files-metadata/:path")
    UpdateFileResponse updateFile(1: UpdateFileRequest req) (api.put="/api/v1/files/:path")
    DeleteFileResponse deleteFile(1: DeleteFileRequest req) (api.delete="/api/v1/files/:path")
    MoveFileResponse moveFile(1: MoveFileRequest req) (api.post="/api/v1/files/move")
    ListFileVersionsResponse listFileVersions(1: ListFileVersionsRequest req) (api.get="/api/v1/files-version/:path")
    
    // File relations
    CreateFileRelationResponse createFileRelation(1: CreateFileRelationRequest req) (api.post="/api/v1/file-relations")
    ListFileRelationsResponse listFileRelations(1: ListFileRelationsRequest req) (api.get="/api/v1/file-relations/:path")
    
    // Workflow operations
    GetWorkflowInfoResponse getWorkflowInfo(1: GetWorkflowInfoRequest req) (api.get="/api/v1/workflows/info/:filepath")
    GetValidTransitionsResponse getValidTransitions(1: GetValidTransitionsRequest req) (api.get="/api/v1/workflows/transitions/:filepath")
    TransitionToStateResponse transitionToState(1: TransitionToStateRequest req) (api.post="/api/v1/workflows/next/:filepath")
    
    // Search operations
    SearchFilesResponse searchFiles(1: SearchFilesRequest req) (api.get="/api/v1/search")
    
    // Auth operations
    LoginResponse login(1: LoginRequest req) (api.post="/api/v1/auth/login")
}