# Lumigo Receiver

This receiver accepts Lumigo-formatted trace spans and converts them to OpenTelemetry Protocol (OTLP) format.

## Overview

The Lumigo receiver is designed to receive trace data from Lumigo's tracing infrastructure and transform it into OTLP spans that can be processed by the OpenTelemetry Collector pipeline.

## Supported Span Types

The receiver currently supports the following Lumigo span types:

- **Function spans**: AWS Lambda function invocations
- **HTTP spans**: HTTP client requests

## Configuration

The receiver accepts the following configuration parameters:

- `endpoint` (default: `0.0.0.0:8088`): The HTTP endpoint to listen on for incoming Lumigo spans

### Example Configuration

```yaml
receivers:
  lumigo:
    endpoint: 0.0.0.0:8088

exporters:
  otlp:
    endpoint: http://backend:4317

service:
  pipelines:
    traces:
      receivers: [lumigo]
      exporters: [otlp]
```

### With Custom Endpoint

```yaml
receivers:
  lumigo:
    endpoint: localhost:9999
```

## Sending Spans

Lumigo spans should be sent as JSON via HTTP POST to the `/v1/traces` endpoint.

### Single Span

```bash
curl -X POST http://localhost:8088/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "id": "span-id-123",
    "transactionId": "transaction-456",
    "type": "function",
    "name": "my-lambda-function",
    "started": 1234567890000,
    "ended": 1234567891000,
    "account": "123456789012",
    "region": "us-east-1"
  }'
```

### Multiple Spans (Batch)

```bash
curl -X POST http://localhost:8088/v1/traces \
  -H "Content-Type: application/json" \
  -d '[
    {
      "id": "span-1",
      "transactionId": "tx-1",
      "type": "function",
      "started": 1000,
      "ended": 2000
    },
    {
      "id": "span-2",
      "parentId": "span-1",
      "transactionId": "tx-1",
      "type": "http",
      "started": 1500,
      "ended": 1800
    }
  ]'
```

## Health Check

The receiver exposes a health check endpoint at `/health`:

```bash
curl http://localhost:8088/health
```

Returns: `{"status":"healthy"}`

## Span Transformation

### Common Fields

All Lumigo spans are transformed with the following mappings:

| Lumigo Field | OTLP Field | Notes |
|--------------|------------|-------|
| `id` | Span ID | UUID converted to 8-byte span ID |
| `parentId` | Parent Span ID | UUID converted to 8-byte span ID |
| `transactionId` / `info.traceId.Root` | Trace ID | Prefers AWS X-Ray trace ID format |
| `started` | Start Timestamp | Converted from milliseconds to nanoseconds |
| `ended` | End Timestamp | Converted from milliseconds to nanoseconds |
| `account` | `cloud.account.id` | Resource attribute |
| `region` | `cloud.region` | Resource attribute |
| `token` | `lumigo.token` | Resource attribute |

### Function Span Attributes

Lambda function spans (`type: "function"`) are transformed with:

**Resource Attributes:**
- `cloud.provider`: `aws`
- `cloud.platform`: `aws_lambda`
- `faas.name`: Function name
- `faas.runtime`: Runtime identifier
- `faas.memory_limit`: Memory allocation
- `aws.log.group.name`: CloudWatch log group
- `aws.log.stream.name`: CloudWatch log stream

**Span Attributes:**
- `lumigo.type`: `function`
- `faas.coldstart`: Cold start status (`cold` or `warm`)
- `faas.return_value`: Function return value
- `faas.trigger`: Trigger type (e.g., `kinesis`, `sqs`, `http`)
- Additional trigger-specific attributes

**Span Kind:** `SERVER`

### HTTP Span Attributes

HTTP client spans (`type: "http"`) are transformed with:

**Span Attributes:**
- `lumigo.type`: `http`
- `http.request.method`: HTTP method
- `http.response.status_code`: HTTP status code
- `server.address`: Target host
- `url.full`: Full URL
- `aws.resource.name`: AWS resource name
- `aws.request.id`: AWS request ID
- `http.request.headers`: Request headers (as JSON string)
- `http.response.headers`: Response headers (as JSON string)

**Span Kind:** `CLIENT`

## Trace ID Handling

The receiver handles AWS X-Ray trace IDs in the format `1-{epoch}-{unique-id}`. These are converted to OTLP trace IDs by combining the epoch and unique ID portions.

## Stability

This receiver is currently in **Alpha** stability level.

## Testing

Run the tests with:

```bash
make test
```

Or directly with Go:

```bash
go test ./...
```
