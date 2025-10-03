namespace go vfs.common

struct RequestContext {
  1: string request_id
  2: optional string actor
}

struct ErrorInfo {
  1: string code
  2: string message
  3: optional map<string, string> details
}

struct DirectoryRef {
  1: string id
  2: string path
}

struct FileRef {
  1: string id
  2: string path
}
