// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.27.0"
	"go.uber.org/zap"
)

func TestUnmarshalLumigoSpans(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "single span",
			input:   `{"id":"test-id","transactionId":"test-tx","type":"function","started":1000,"ended":2000}`,
			want:    1,
			wantErr: false,
		},
		{
			name:    "array of spans",
			input:   `[{"id":"test-id-1","transactionId":"test-tx","type":"function","started":1000,"ended":2000},{"id":"test-id-2","transactionId":"test-tx","type":"http","started":1500,"ended":2500}]`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "started span filtered out",
			input:   `{"id":"test-id_started","transactionId":"test-tx","type":"function","started":1000,"ended":1000}`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "array with started span filtered",
			input:   `[{"id":"test-id-1_started","transactionId":"test-tx","type":"function","started":1000,"ended":1000},{"id":"test-id-2","transactionId":"test-tx","type":"http","started":1500,"ended":2500}]`,
			want:    1,
			wantErr: false,
		},
		{
			name:    "invalid json",
			input:   `{invalid}`,
			want:    0,
			wantErr: true,
		},
		{
			name:    "unsupported span type skipped",
			input:   `{"id":"test-id","transactionId":"test-tx","type":"unsupported","started":1000,"ended":2000}`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "array with unsupported span type skipped",
			input:   `[{"id":"test-id-1","transactionId":"test-tx","type":"function","started":1000,"ended":2000},{"id":"test-id-2","transactionId":"test-tx","type":"unsupported","started":1500,"ended":2500},{"id":"test-id-3","transactionId":"test-tx","type":"http","started":2000,"ended":3000}]`,
			want:    2,
			wantErr: false,
		},
	}

	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans, err := unmarshalLumigoSpans([]byte(tt.input), logger)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, spans, tt.want)
			}
		})
	}
}

func TestTransformLumigoToOTLP_FunctionSpan(t *testing.T) {
	lumigoSpan := &FunctionSpan{
		BaseSpan: BaseSpan{
			ID:            "e4974d95-4c31-47b1-aa24-1cae84dafa9b",
			ParentID:      "",
			TransactionID: "2e14f9b22f07f915f6d6a499",
			Type:          "function",
			Started:       1763895471480,
			Ended:         1763895473429,
			Account:       "114300393969",
			Region:        "us-west-2",
			Token:         "t_1b8e3e1eada1064d41ff",
		},
		Name:            "prod_lumigo-search-engine_push-to-clickhouse",
		Runtime:         "AWS_Lambda_python3.11",
		MemoryAllocated: "2000",
		Readiness:       "cold",
		ReturnValue:     "53",
		Event:           `{"key":"value"}`,
		Info: map[string]interface{}{
			"traceId": map[string]interface{}{
				"Root": "1-6922e8ad-2e14f9b22f07f915f6d6a499",
			},
			"logGroupName":  "/aws/lambda/test-function",
			"logStreamName": "2025/11/23/[$LATEST]test",
			"tracer": map[string]interface{}{
				"version": "1.1.257",
			},
		},
	}

	traces, err := transformLumigoToOTLP(LumigoSpanBatch{lumigoSpan})
	require.NoError(t, err)

	assert.Equal(t, 1, traces.ResourceSpans().Len())
	rs := traces.ResourceSpans().At(0)

	// Check resource attributes
	resourceAttrs := rs.Resource().Attributes()
	val, ok := resourceAttrs.Get(semconv.AttributeCloudProvider)
	assert.True(t, ok)
	assert.Equal(t, semconv.AttributeCloudProviderAWS, val.Str())

	val, ok = resourceAttrs.Get(semconv.AttributeCloudAccountID)
	assert.True(t, ok)
	assert.Equal(t, "114300393969", val.Str())

	val, ok = resourceAttrs.Get(semconv.AttributeCloudRegion)
	assert.True(t, ok)
	assert.Equal(t, "us-west-2", val.Str())

	val, ok = resourceAttrs.Get(semconv.AttributeFaaSName)
	assert.True(t, ok)
	assert.Equal(t, "prod_lumigo-search-engine_push-to-clickhouse", val.Str())

	// Check scope
	assert.Equal(t, 1, rs.ScopeSpans().Len())
	scope := rs.ScopeSpans().At(0).Scope()
	assert.Equal(t, "lumigo-tracer", scope.Name())
	assert.Equal(t, "1.1.257", scope.Version())

	// Check span
	assert.Equal(t, 1, rs.ScopeSpans().At(0).Spans().Len())
	span := rs.ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, "prod_lumigo-search-engine_push-to-clickhouse", span.Name())
	assert.Equal(t, ptrace.SpanKindServer, span.Kind())

	// Check span attributes
	spanAttrs := span.Attributes()
	val, ok = spanAttrs.Get("faas.coldstart")
	assert.True(t, ok)
	assert.Equal(t, "cold", val.Str())

	val, ok = spanAttrs.Get("faas.return_value")
	assert.True(t, ok)
	assert.Equal(t, "53", val.Str())

	val, ok = spanAttrs.Get("faas.event")
	assert.True(t, ok)
	assert.Equal(t, `{"key":"value"}`, val.Str())

	// Verify span status is OK (no error)
	assert.Equal(t, ptrace.StatusCodeOk, span.Status().Code())
	assert.Equal(t, "", span.Status().Message())
	assert.Equal(t, 0, span.Events().Len())
}

func TestTransformLumigoToOTLP_FunctionSpanWithError(t *testing.T) {
	lumigoSpan := &FunctionSpan{
		BaseSpan: BaseSpan{
			ID:            "aa4bb0cf-c686-4b0c-9571-3462b195e3f7",
			ParentID:      "",
			TransactionID: "2ab51b9462dd7bf217a6b71c",
			Type:          "function",
			Started:       1766923829902,
			Ended:         1766923830006,
			Account:       "593621721102",
			Region:        "us-west-2",
			Token:         "t_276e76232b3d49948d76c",
		},
		Name:            "wildrydes-dev-purchaseNewUnicron",
		Runtime:         "AWS_Lambda_nodejs18.x",
		MemoryAllocated: "1024",
		Readiness:       "warm",
		ReturnValue:     "null",
		Error: &SpanError{
			Type:       "Error",
			Message:    "/var/task/node_modules/snappy/build/Release/binding.node: invalid ELF header",
			Stacktrace: "Error: /var/task/node_modules/snappy/build/Release/binding.node: invalid ELF header\n    at Module._extensions..node (node:internal/modules/cjs/loader:1460:18)\n    at Module.load (node:internal/modules/cjs/loader:1203:32)",
		},
		Info: map[string]interface{}{
			"traceId": map[string]interface{}{
				"Root": "1-69511e35-2ab51b9462dd7bf217a6b71c",
			},
		},
	}

	traces, err := transformLumigoToOTLP(LumigoSpanBatch{lumigoSpan})
	require.NoError(t, err)

	assert.Equal(t, 1, traces.ResourceSpans().Len())
	rs := traces.ResourceSpans().At(0)

	// Check span
	assert.Equal(t, 1, rs.ScopeSpans().Len())
	assert.Equal(t, 1, rs.ScopeSpans().At(0).Spans().Len())
	span := rs.ScopeSpans().At(0).Spans().At(0)

	// Verify span status is ERROR
	assert.Equal(t, ptrace.StatusCodeError, span.Status().Code())
	assert.Equal(t, "/var/task/node_modules/snappy/build/Release/binding.node: invalid ELF header", span.Status().Message())

	// Verify error attributes
	spanAttrs := span.Attributes()
	val, ok := spanAttrs.Get("error.type")
	assert.True(t, ok)
	assert.Equal(t, "Error", val.Str())

	val, ok = spanAttrs.Get("error.message")
	assert.True(t, ok)
	assert.Equal(t, "/var/task/node_modules/snappy/build/Release/binding.node: invalid ELF header", val.Str())

	val, ok = spanAttrs.Get("error.stack")
	assert.True(t, ok)
	assert.Contains(t, val.Str(), "Module._extensions..node")

	// Verify exception event
	assert.Equal(t, 1, span.Events().Len())
	event := span.Events().At(0)
	assert.Equal(t, "exception", event.Name())
	assert.Equal(t, span.EndTimestamp(), event.Timestamp())

	// Verify exception event attributes
	eventAttrs := event.Attributes()
	val, ok = eventAttrs.Get("exception.type")
	assert.True(t, ok)
	assert.Equal(t, "Error", val.Str())

	val, ok = eventAttrs.Get("exception.message")
	assert.True(t, ok)
	assert.Equal(t, "/var/task/node_modules/snappy/build/Release/binding.node: invalid ELF header", val.Str())

	val, ok = eventAttrs.Get("exception.stacktrace")
	assert.True(t, ok)
	assert.Contains(t, val.Str(), "Module._extensions..node")
}

func TestTransformLumigoToOTLP_HTTPSpan(t *testing.T) {
	lumigoSpan := &HTTPSpan{
		BaseSpan: BaseSpan{
			ID:            "e1b519c1-c819-44c1-8a7d-42f20f10c46b",
			ParentID:      "e4974d95-4c31-47b1-aa24-1cae84dafa9b",
			TransactionID: "2e14f9b22f07f915f6d6a499",
			Type:          "http",
			Started:       1763895471750,
			Ended:         1763895471812,
			Account:       "114300393969",
			Region:        "us-west-2",
			Token:         "t_1b8e3e1eada1064d41ff",
		},
		Info: map[string]interface{}{
			"httpInfo": map[string]interface{}{
				"request": map[string]interface{}{
					"method": "GET",
					"uri":    "lmg-prod-common-resources-config-cache.s3.us-west-2.amazonaws.com/customers/all_customers",
				},
				"host": "lmg-prod-common-resources-config-cache.s3.us-west-2.amazonaws.com",
				"response": map[string]interface{}{
					"statusCode": float64(200),
				},
			},
			"resourceName": "lmg-prod-common-resources-config-cache",
			"messageId":    "G4K740EBCCK2DTMG",
			"traceId": map[string]interface{}{
				"Root": "1-6922e8ad-2e14f9b22f07f915f6d6a499",
			},
		},
	}

	traces, err := transformLumigoToOTLP(LumigoSpanBatch{lumigoSpan})
	require.NoError(t, err)

	assert.Equal(t, 1, traces.ResourceSpans().Len())
	rs := traces.ResourceSpans().At(0)

	// Check span
	assert.Equal(t, 1, rs.ScopeSpans().Len())
	assert.Equal(t, 1, rs.ScopeSpans().At(0).Spans().Len())
	span := rs.ScopeSpans().At(0).Spans().At(0)

	assert.Equal(t, "GET lmg-prod-common-resources-config-cache.s3.us-west-2.amazonaws.com/customers/all_customers", span.Name())
	assert.Equal(t, ptrace.SpanKindClient, span.Kind())

	// Check HTTP attributes
	spanAttrs := span.Attributes()
	val, ok := spanAttrs.Get(semconv.AttributeHTTPRequestMethod)
	assert.True(t, ok)
	assert.Equal(t, "GET", val.Str())

	val, ok = spanAttrs.Get(semconv.AttributeHTTPResponseStatusCode)
	assert.True(t, ok)
	assert.Equal(t, int64(200), val.Int())

	val, ok = spanAttrs.Get(semconv.AttributeServerAddress)
	assert.True(t, ok)
	assert.Equal(t, "lmg-prod-common-resources-config-cache.s3.us-west-2.amazonaws.com", val.Str())

	val, ok = spanAttrs.Get("aws.resource.name")
	assert.True(t, ok)
	assert.Equal(t, "lmg-prod-common-resources-config-cache", val.Str())
}

func TestTransformLumigoToOTLP_WithRealSpans(t *testing.T) {
	// Test with real span files if they exist
	spanFiles := []string{
		"testdata/spans/function_span.json",
		"testdata/spans/http_span.json",
	}

	for _, spanFile := range spanFiles {
		if _, err := os.Stat(spanFile); os.IsNotExist(err) {
			t.Logf("Skipping test with %s (file doesn't exist)", spanFile)
			continue
		}

		t.Run(filepath.Base(spanFile), func(t *testing.T) {
			data, err := os.ReadFile(spanFile)
			require.NoError(t, err)

			logger := zap.NewNop()
			spans, err := unmarshalLumigoSpans(data, logger)
			require.NoError(t, err)
			require.NotEmpty(t, spans)

			traces, err := transformLumigoToOTLP(spans)
			require.NoError(t, err)

			assert.Greater(t, traces.ResourceSpans().Len(), 0)
		})
	}
}

func TestParseSpanID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			id:      "e4974d95-4c31-47b1-aa24-1cae84dafa9b",
			wantErr: false,
		},
		{
			name:    "short ID",
			id:      "abc123",
			wantErr: false, // Should pad with zeros
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spanID, err := parseSpanID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, [8]byte{}, spanID)
			}
		})
	}
}

func TestParseTraceID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "AWS X-Ray format",
			id:      "1-6922e8ad-2e14f9b22f07f915f6d6a499",
			wantErr: false,
		},
		{
			name:    "regular hex string",
			id:      "2e14f9b22f07f915f6d6a499",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceID, err := parseTraceID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, [16]byte{}, traceID)
			}
		})
	}
}

func TestGetSpanName(t *testing.T) {
	tests := []struct {
		name string
		span LumigoSpan
		want string
	}{
		{
			name: "function span with name",
			span: &FunctionSpan{
				BaseSpan: BaseSpan{Type: "function"},
				Name:     "my-lambda-function",
			},
			want: "my-lambda-function",
		},
		{
			name: "function span without name",
			span: &FunctionSpan{
				BaseSpan: BaseSpan{Type: "function"},
			},
			want: "AWS Lambda Invocation",
		},
		{
			name: "http span",
			span: &HTTPSpan{
				BaseSpan: BaseSpan{Type: "http"},
				Info: map[string]interface{}{
					"httpInfo": map[string]interface{}{
						"request": map[string]interface{}{
							"method": "POST",
							"uri":    "/api/users",
						},
					},
				},
			},
			want: "POST /api/users",
		},
		{
			name: "http span without details",
			span: &HTTPSpan{
				BaseSpan: BaseSpan{Type: "http"},
			},
			want: "HTTP Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSpanName(tt.span)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetSpanKind(t *testing.T) {
	tests := []struct {
		name string
		span LumigoSpan
		want ptrace.SpanKind
	}{
		{
			name: "function span",
			span: &FunctionSpan{BaseSpan: BaseSpan{Type: "function"}},
			want: ptrace.SpanKindServer,
		},
		{
			name: "http span",
			span: &HTTPSpan{BaseSpan: BaseSpan{Type: "http"}},
			want: ptrace.SpanKindClient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSpanKind(tt.span)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractTraceID(t *testing.T) {
	tests := []struct {
		name string
		span LumigoSpan
		want string
	}{
		{
			name: "with X-Ray trace ID",
			span: &FunctionSpan{
				BaseSpan: BaseSpan{TransactionID: "tx-123"},
				Info: map[string]interface{}{
					"traceId": map[string]interface{}{
						"Root": "1-6922e8ad-2e14f9b22f07f915f6d6a499",
					},
				},
			},
			want: "1-6922e8ad-2e14f9b22f07f915f6d6a499",
		},
		{
			name: "without X-Ray trace ID",
			span: &FunctionSpan{
				BaseSpan: BaseSpan{TransactionID: "tx-123"},
			},
			want: "tx-123",
		},
		{
			name: "fallback to span ID",
			span: &HTTPSpan{
				BaseSpan: BaseSpan{ID: "span-456"},
			},
			want: "span-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTraceID(tt.span)
			assert.Equal(t, tt.want, got)
		})
	}
}
