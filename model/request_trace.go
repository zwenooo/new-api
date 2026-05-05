package model

// RequestTraceSession + RequestTraceNode provide an indexed view for full request/response
// traces whose large payloads are stored outside SQL (filesystem).

type RequestTraceSession struct {
	RequestId     string `gorm:"primaryKey;type:varchar(64);column:request_id" json:"request_id"`
	CreatedAt     int64  `gorm:"bigint;not null;index;column:created_at" json:"created_at"`
	UpdatedAt     int64  `gorm:"bigint;not null;index;column:updated_at" json:"updated_at"`
	RequestMethod string `gorm:"type:varchar(16);not null;default:'';column:request_method" json:"request_method"`
	RequestPath   string `gorm:"type:varchar(1024);not null;default:'';column:request_path" json:"request_path"`
}

func (RequestTraceSession) TableName() string {
	return "request_trace_sessions"
}

type RequestTraceNode struct {
	Id        int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RequestId string `gorm:"type:varchar(64);not null;index:idx_request_trace_req_service_started,priority:1;column:request_id" json:"request_id"`
	Service   string `gorm:"type:varchar(64);not null;index:idx_request_trace_req_service_started,priority:2;column:service" json:"service"`
	Kind      string `gorm:"type:varchar(64);not null;default:'';column:kind" json:"kind"`
	Seq       int    `gorm:"not null;default:0;column:seq" json:"seq"`

	StartedAt int64 `gorm:"bigint;not null;index:idx_request_trace_req_service_started,priority:3;column:started_at" json:"started_at"`
	EndedAt   int64 `gorm:"bigint;not null;default:0;column:ended_at" json:"ended_at"`

	RequestMethod string `gorm:"type:varchar(16);not null;default:'';column:request_method" json:"request_method"`
	RequestURL    string `gorm:"type:text;column:request_url" json:"request_url"`
	RequestPath   string `gorm:"type:varchar(2048);not null;default:'';column:request_path" json:"request_path"`

	RequestHeadersKey  string `gorm:"type:text;column:request_headers_key" json:"request_headers_key"`
	RequestHeadersSize int64  `gorm:"bigint;not null;default:0;column:request_headers_size" json:"request_headers_size"`
	RequestBodyKey     string `gorm:"type:text;column:request_body_key" json:"request_body_key"`
	RequestBodySize    int64  `gorm:"bigint;not null;default:0;column:request_body_size" json:"request_body_size"`

	ResponseStatus      int    `gorm:"not null;default:0;column:response_status" json:"response_status"`
	ResponseHeadersKey  string `gorm:"type:text;column:response_headers_key" json:"response_headers_key"`
	ResponseHeadersSize int64  `gorm:"bigint;not null;default:0;column:response_headers_size" json:"response_headers_size"`
	ResponseBodyKey     string `gorm:"type:text;column:response_body_key" json:"response_body_key"`
	ResponseBodySize    int64  `gorm:"bigint;not null;default:0;column:response_body_size" json:"response_body_size"`

	Error string `gorm:"type:text;column:error" json:"error"`
	Meta  string `gorm:"type:longtext;column:meta" json:"meta"`

	CreatedAt int64 `gorm:"bigint;not null;index;column:created_at" json:"created_at"`
	UpdatedAt int64 `gorm:"bigint;not null;index;column:updated_at" json:"updated_at"`
}

func (RequestTraceNode) TableName() string {
	return "request_trace_nodes"
}
