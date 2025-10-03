include "common.thrift"

namespace go vfs.content

struct InitiateUploadRequest {
  1: common.RequestContext context
  2: string file_id
  3: optional i64 size
  4: optional string checksum
}

struct InitiateUploadResponse {
  1: string upload_id
  2: optional common.ErrorInfo error
}

struct UploadChunkRequest {
  1: common.RequestContext context
  2: string upload_id
  3: i64 sequence
  4: binary data
  5: optional string checksum
}

struct UploadCompleteRequest {
  1: common.RequestContext context
  2: string upload_id
}

struct UploadCompleteResponse {
  1: string content_pointer
  2: optional common.ErrorInfo error
}

struct DownloadContentRequest {
  1: common.RequestContext context
  2: string content_pointer
}

struct DownloadContentResponse {
  1: binary data
  2: optional common.ErrorInfo error
}

service ContentService {
  InitiateUploadResponse InitiateUpload(1: InitiateUploadRequest req)
  common.ErrorInfo UploadChunk(1: UploadChunkRequest req)
  UploadCompleteResponse CompleteUpload(1: UploadCompleteRequest req)
  DownloadContentResponse DownloadContent(1: DownloadContentRequest req)
}
