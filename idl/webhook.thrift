include "common.thrift"

namespace go vfs.webhook

struct WebhookDelivery {
  1: string delivery_id
  2: string event_id
  3: string target_url
  4: string payload
  5: i64 created_at
}

struct AckWebhookRequest {
  1: common.RequestContext context
  2: string delivery_id
  3: bool success
  4: optional string response_payload
  5: optional string error_message
}

struct AckWebhookResponse {
  1: optional common.ErrorInfo error
}

service WebhookService {
  AckWebhookResponse AckWebhook(1: AckWebhookRequest req)
}
