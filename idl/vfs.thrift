namespace go vfs

// Common types
struct Timestamp {
    1: required i64 seconds
    2: required i32 nanos
}

struct Empty {}

// Directory types
struct Directory {
    1: required string id
    2: optional string parent_id
    3: required string name
    4: required string path
    5: required i64 version
    6: optional string opa_policy_id
    7: required Timestamp created_at
    8: required Timestamp updated_at
    9: optional Timestamp deleted_at
}

struct CreateDirectoryRequest {
    1: required string parent_path
    2: required string name
    3: optional string opa_policy_id
}

struct CreateDirectoryResponse {
    1: required Directory directory
}

struct ListDirectoryRequest {
    1: required string path
    2: optional i32 limit
    3: optional string cursor
}

struct DirectoryEntry {
    1: required string name
    2: required string type // "directory" or "file"
    3: required i64 size_bytes // 0 for directories
    4: required Timestamp modified_at
}

struct ListDirectoryResponse {
    1: required list<DirectoryEntry> entries
    2: optional string next_cursor
}

struct DeleteDirectoryRequest {
    1: required string path
    2: required bool recursive
}

struct DeleteDirectoryResponse {
    1: required bool success
}

// File types
enum StorageType {
    JSON = 1,
    S3 = 2
}

struct File {
    1: required string id
    2: required string directory_id
    3: required string name
    4: required string content_type
    5: required i64 size_bytes
    6: required StorageType storage_type
    7: optional string json_content
    8: optional string s3_key
    9: required string checksum_sha256
    10: required i64 version
    11: required Timestamp created_at
    12: required Timestamp updated_at
    13: optional Timestamp deleted_at
}

struct CreateFileRequest {
    1: required string directory_path
    2: required string name
    3: required string content_type
    4: required i64 size_bytes
    5: required binary content
    6: required string checksum_sha256
}

struct CreateFileResponse {
    1: required File file
}

struct GetFileRequest {
    1: required string path
    2: optional i64 version // if not specified, get latest
}

struct GetFileResponse {
    1: required File file
    2: required binary content
}

struct UpdateFileRequest {
    1: required string path
    2: required string content_type
    3: required i64 size_bytes
    4: required binary content
    5: required string checksum_sha256
    6: required i64 expected_version // optimistic locking
}

struct UpdateFileResponse {
    1: required File file
}

struct DeleteFileRequest {
    1: required string path
}

struct DeleteFileResponse {
    1: required bool success
}

struct MoveFileRequest {
    1: required string source_path
    2: required string destination_path
}

struct MoveFileResponse {
    1: required File file
}

struct GetFileMetadataRequest {
    1: required string path
}

struct GetFileMetadataResponse {
    1: required File file
}

// File relations (derivatives)
struct FileRelation {
    1: required string id
    2: required string parent_file_id
    3: required string derivative_file_id
    4: required string relation_type
    5: optional string metadata_json
    6: required Timestamp created_at
}

struct CreateFileRelationRequest {
    1: required string parent_file_path
    2: required string derivative_file_path
    3: required string relation_type
    4: optional string metadata_json
}

struct CreateFileRelationResponse {
    1: required FileRelation relation
}

struct ListFileRelationsRequest {
    1: required string file_path
    2: required string direction // "parents" or "derivatives"
}

struct ListFileRelationsResponse {
    1: required list<FileRelation> relations
}

// File versions
struct FileVersion {
    1: required string id
    2: required string file_id
    3: required i64 version_number
    4: required string content_type
    5: required i64 size_bytes
    6: required StorageType storage_type
    7: optional string json_content
    8: optional string s3_key
    9: required string checksum_sha256
    10: required Timestamp created_at
}

struct ListFileVersionsRequest {
    1: required string file_path
    2: optional i32 limit
}

struct ListFileVersionsResponse {
    1: required list<FileVersion> versions
}

// Health check
struct HealthCheckRequest {}

struct HealthCheckResponse {
    1: required string status // "ok" or "degraded"
    2: optional string message
    3: optional map<string, string> checks // component -> status
}

// Exceptions
exception ValidationException {
    1: required string message
    2: optional map<string, string> field_errors
}

exception NotFoundException {
    1: required string message
    2: required string resource_type
    3: required string resource_id
}

exception ConflictException {
    1: required string message
    2: optional string conflicting_resource_id
}

exception UnauthorizedException {
    1: required string message
}

exception InternalException {
    1: required string message
    2: optional string trace_id
}

exception IdempotencyException {
    1: required string message
    2: required string request_id
    3: optional string cached_response_json
}

// VFS Service
service VFSService {
    // Health
    HealthCheckResponse health(1: HealthCheckRequest req)

    // Directories
    CreateDirectoryResponse createDirectory(1: CreateDirectoryRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    ListDirectoryResponse listDirectory(1: ListDirectoryRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr)

    DeleteDirectoryResponse deleteDirectory(1: DeleteDirectoryRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    // Files
    CreateFileResponse createFile(1: CreateFileRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    GetFileResponse getFile(1: GetFileRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr)

    UpdateFileResponse updateFile(1: UpdateFileRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    DeleteFileResponse deleteFile(1: DeleteFileRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    MoveFileResponse moveFile(1: MoveFileRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    GetFileMetadataResponse getFileMetadata(1: GetFileMetadataRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr)

    // File Relations
    CreateFileRelationResponse createFileRelation(1: CreateFileRelationRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: ConflictException conflictErr,
                4: UnauthorizedException unauthorizedErr,
                5: InternalException internalErr,
                6: IdempotencyException idempotencyErr)

    ListFileRelationsResponse listFileRelations(1: ListFileRelationsRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr)

    // File Versions
    ListFileVersionsResponse listFileVersions(1: ListFileVersionsRequest req)
        throws (1: ValidationException validationErr,
                2: NotFoundException notFoundErr,
                3: UnauthorizedException unauthorizedErr,
                4: InternalException internalErr)
}
