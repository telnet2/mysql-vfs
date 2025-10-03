include "common.thrift"

namespace go vfs.scheduler

struct CronPayload {
  1: string directory_id
  2: string cron_expr
  3: string payload
  4: optional string timezone
}

struct RegisterCronRequest {
  1: common.RequestContext context
  2: CronPayload payload
}

struct RegisterCronResponse {
  1: string cron_job_id
  2: optional common.ErrorInfo error
}

struct ListCronRequest {
  1: common.RequestContext context
  2: string directory_id
}

struct CronEntry {
  1: string id
  2: string cron_expr
  3: string payload
  4: optional string timezone
  5: i64 created_at
  6: i64 updated_at
}

struct ListCronResponse {
  1: list<CronEntry> cron_jobs
  2: optional common.ErrorInfo error
}

struct TriggerCronRequest {
  1: common.RequestContext context
  2: string cron_job_id
  3: string execution_key
}

struct TriggerCronResponse {
  1: optional common.ErrorInfo error
}

service SchedulerService {
  RegisterCronResponse RegisterCron(1: RegisterCronRequest req)
  ListCronResponse ListCron(1: ListCronRequest req)
  TriggerCronResponse TriggerCron(1: TriggerCronRequest req)
}
