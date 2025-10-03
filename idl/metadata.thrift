include "common.thrift"

namespace go vfs.metadata

struct DirectoryPayload {
  1: string name
  2: optional string parent_id
}

struct FilePayload {
  1: string name
  2: string directory_id
  3: optional string origin_file_id
  4: optional string checksum
  5: optional i64 size
  6: optional string mime_type
}

struct DirectoryEntry {
  1: string id
  2: string name
  3: optional string parent_id
  4: string path
  5: i64 version
  6: i64 created_at
  7: i64 updated_at
}

struct FileEntry {
  1: string id
  2: string name
  3: string directory_id
  4: optional string origin_file_id
  5: i64 version
  6: optional string checksum
  7: optional i64 size
  8: optional string mime_type
  9: i64 created_at
  10: i64 updated_at
}

struct ListDirectoryRequest {
  1: common.RequestContext context
  2: string directory_id
  3: bool recursive = false
}

struct ListDirectoryResponse {
  1: list<DirectoryEntry> directories
  2: list<FileEntry> files
  3: optional common.ErrorInfo error
}

struct CreateDirectoryRequest {
  1: common.RequestContext context
  2: DirectoryPayload payload
}

struct CreateDirectoryResponse {
  1: DirectoryEntry directory
  2: optional common.ErrorInfo error
}

struct CreateFileRequest {
  1: common.RequestContext context
  2: FilePayload payload
  3: optional string content_pointer
}

struct CreateFileResponse {
  1: FileEntry file
  2: optional common.ErrorInfo error
}

struct UpdateFileRequest {
  1: common.RequestContext context
  2: string file_id
  3: optional FilePayload payload
  4: optional string content_pointer
  5: optional i64 expected_version
}

struct UpdateFileResponse {
  1: FileEntry file
  2: optional common.ErrorInfo error
}

struct MoveEntryRequest {
  1: common.RequestContext context
  2: string entry_id
  3: bool is_directory
  4: string target_directory_id
  5: optional string new_name
  6: optional i64 expected_version
}

struct DeleteEntryRequest {
  1: common.RequestContext context
  2: string entry_id
  3: bool is_directory
  4: optional i64 expected_version
}

struct BasicResponse {
  1: optional common.ErrorInfo error
}

struct GetFileRequest {
  1: common.RequestContext context
  2: string file_id
}

struct GetFileResponse {
  1: FileEntry file
  2: optional string content_pointer
  3: optional common.ErrorInfo error
}

service MetadataService {
  ListDirectoryResponse ListDirectory(1: ListDirectoryRequest req)
  CreateDirectoryResponse CreateDirectory(1: CreateDirectoryRequest req)
  CreateFileResponse CreateFile(1: CreateFileRequest req)
  UpdateFileResponse UpdateFile(1: UpdateFileRequest req)
  BasicResponse MoveEntry(1: MoveEntryRequest req)
  BasicResponse DeleteEntry(1: DeleteEntryRequest req)
  GetFileResponse GetFile(1: GetFileRequest req)
}
