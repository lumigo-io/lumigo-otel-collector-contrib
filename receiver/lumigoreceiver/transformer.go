// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver // import "github.com/lumigo-io/lumigo-otel-collector-contrib/receiver/lumigoreceiver"

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.27.0"
	"go.uber.org/zap"
)

// ErrSkipSpan is returned when a span should be skipped during unmarshaling
var ErrSkipSpan = errors.New("span should be skipped")

// BaseSpan contains fields common to all span types
type BaseSpan struct {
	ID                       string `json:"id"`
	ParentID                 string `json:"parentId"`
	TransactionID            string `json:"transactionId"`
	Type                     string `json:"type"`
	Started                  int64  `json:"started"`
	Ended                    int64  `json:"ended"`
	Account                  string `json:"account,omitempty"`
	Region                   string `json:"region,omitempty"`
	Token                    string `json:"token,omitempty"`
	LambdaContainerID        string `json:"lambda_container_id,omitempty"`
	IsMalformedTransactionID bool   `json:"isMalformedTransactionId,omitempty"`
}

// SpanError represents error information in a span
type SpanError struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Stacktrace string `json:"stacktrace"`
}

// FunctionSpan represents a Lambda function span
type FunctionSpan struct {
	BaseSpan
	Name            string                 `json:"name,omitempty"`
	Runtime         string                 `json:"runtime,omitempty"`
	MemoryAllocated string                 `json:"memoryAllocated,omitempty"`
	Readiness       string                 `json:"readiness,omitempty"`
	ReturnValue     string                 `json:"return_value,omitempty"`
	Event           string                 `json:"event,omitempty"`
	Envs            string                 `json:"envs,omitempty"`
	ReporterRTT     int                    `json:"reporter_rtt,omitempty"`
	Error           *SpanError             `json:"error,omitempty"`
	Info            map[string]interface{} `json:"info,omitempty"`
}

// HTTPSpan represents an HTTP request span
type HTTPSpan struct {
	BaseSpan
	Info map[string]interface{} `json:"info,omitempty"`
}

// LumigoSpan is an interface that all span types implement
type LumigoSpan interface {
	GetBaseSpan() *BaseSpan
	GetType() string
	GetInfo() map[string]interface{}
}

// GetBaseSpan returns the base span for FunctionSpan
func (s *FunctionSpan) GetBaseSpan() *BaseSpan {
	return &s.BaseSpan
}

// GetType returns the span type for FunctionSpan
func (s *FunctionSpan) GetType() string {
	return s.Type
}

// GetInfo returns the info map for FunctionSpan
func (s *FunctionSpan) GetInfo() map[string]interface{} {
	return s.Info
}

// GetBaseSpan returns the base span for HTTPSpan
func (s *HTTPSpan) GetBaseSpan() *BaseSpan {
	return &s.BaseSpan
}

// GetType returns the span type for HTTPSpan
func (s *HTTPSpan) GetType() string {
	return s.Type
}

// GetInfo returns the info map for HTTPSpan
func (s *HTTPSpan) GetInfo() map[string]interface{} {
	return s.Info
}

// LumigoSpanBatch represents a batch of Lumigo spans
type LumigoSpanBatch []LumigoSpan

// transformLumigoToOTLP converts Lumigo spans to OTLP traces
func transformLumigoToOTLP(lumigoSpans LumigoSpanBatch) (ptrace.Traces, error) {
	traces := ptrace.NewTraces()

	// Group spans by trace ID
	traceMap := make(map[string][]LumigoSpan)
	for _, span := range lumigoSpans {
		traceID := extractTraceID(span)
		traceMap[traceID] = append(traceMap[traceID], span)
	}

	// Create resource spans for each trace
	for _, spans := range traceMap {
		if len(spans) == 0 {
			continue
		}

		rs := traces.ResourceSpans().AppendEmpty()
		resource := rs.Resource()

		// Find the function span for resource attributes (prioritize over HTTP spans)
		// This ensures service.name and other Lambda-specific attrs are set correctly
		resourceSpan := findResourceSpan(spans)
		setResourceAttributes(resource.Attributes(), resourceSpan)

		scopeSpans := rs.ScopeSpans().AppendEmpty()
		scopeSpans.Scope().SetName("lumigo-tracer")

		// Extract version from tracer info if available
		spanInfo := spans[0].GetInfo()
		if spanInfo != nil {
			if info, ok := spanInfo["tracer"].(map[string]interface{}); ok {
				if version, ok := info["version"].(string); ok {
					scopeSpans.Scope().SetVersion(version)
				}
			}
		}

		// Convert each Lumigo span to OTLP span
		for _, lumigoSpan := range spans {
			otlpSpan := scopeSpans.Spans().AppendEmpty()
			if err := convertSpan(lumigoSpan, otlpSpan); err != nil {
				return traces, fmt.Errorf("failed to convert span: %w", err)
			}
		}
	}

	return traces, nil
}

// findResourceSpan finds the best span to use for resource attributes
// Prioritizes function spans over HTTP spans for proper service naming
func findResourceSpan(spans []LumigoSpan) LumigoSpan {
	for _, span := range spans {
		if _, ok := span.(*FunctionSpan); ok {
			return span
		}
	}
	// Fallback to first span if no function span found
	return spans[0]
}

// extractTraceID extracts the trace ID from a Lumigo span
func extractTraceID(span LumigoSpan) string {
	base := span.GetBaseSpan()
	info := span.GetInfo()

	// Try to get AWS X-Ray trace ID from info.traceId.Root
	if info != nil {
		if traceIDInfo, ok := info["traceId"].(map[string]interface{}); ok {
			if root, ok := traceIDInfo["Root"].(string); ok && root != "" {
				return root
			}
		}
	}

	// Fallback to transactionId
	if base.TransactionID != "" {
		return base.TransactionID
	}

	// Last resort: use the span ID
	return base.ID
}

// setResourceAttributes sets resource-level attributes
func setResourceAttributes(attrs pcommon.Map, span LumigoSpan) {
	base := span.GetBaseSpan()
	info := span.GetInfo()

	// Cloud provider
	attrs.PutStr(semconv.AttributeCloudProvider, semconv.AttributeCloudProviderAWS)

	if base.Account != "" {
		attrs.PutStr(semconv.AttributeCloudAccountID, base.Account)
	}

	if base.Region != "" {
		attrs.PutStr(semconv.AttributeCloudRegion, base.Region)
	}

	// AWS Lambda specific attributes for function spans
	if funcSpan, ok := span.(*FunctionSpan); ok {
		attrs.PutStr(semconv.AttributeCloudPlatform, semconv.AttributeCloudPlatformAWSLambda)

		if funcSpan.Name != "" {
			attrs.PutStr(semconv.AttributeFaaSName, funcSpan.Name)
		}

		if funcSpan.MemoryAllocated != "" {
			attrs.PutStr(semconv.AttributeFaaSMaxMemory, funcSpan.MemoryAllocated)
		}

		if funcSpan.Runtime != "" {
			attrs.PutStr("faas.runtime", funcSpan.Runtime)
		}

		// Build Lambda ARN for cloud.resource_id
		if base.Account != "" && base.Region != "" && funcSpan.Name != "" {
			arn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", base.Region, base.Account, funcSpan.Name)
			attrs.PutStr(semconv.AttributeCloudResourceID, arn)
		}

		// Add Lambda-specific info from Info field
		if info != nil {
			if logGroupName, ok := info["logGroupName"].(string); ok {
				attrs.PutStr("aws.log.group.name", logGroupName)
			}
			if logStreamName, ok := info["logStreamName"].(string); ok {
				attrs.PutStr("aws.log.stream.name", logStreamName)
			}
		}

		// Service name for function
		if funcSpan.Name != "" {
			attrs.PutStr(semconv.AttributeServiceName, funcSpan.Name)
		} else {
			attrs.PutStr(semconv.AttributeServiceName, "unknown-service")
		}
	} else {
		// For non-function spans
		attrs.PutStr(semconv.AttributeServiceName, "unknown-service")
	}

	// Lumigo-specific attributes
	if base.Token != "" {
		attrs.PutStr("lumigo.token", base.Token)
	}
}

// convertSpan converts a single Lumigo span to OTLP span
func convertSpan(lumigoSpan LumigoSpan, otlpSpan ptrace.Span) error {
	base := lumigoSpan.GetBaseSpan()

	// Set span ID
	spanID, err := parseSpanID(base.ID)
	if err != nil {
		return fmt.Errorf("invalid span ID: %w", err)
	}
	otlpSpan.SetSpanID(spanID)

	// Set parent span ID
	if base.ParentID != "" {
		parentID, err := parseSpanID(base.ParentID)
		if err != nil {
			return fmt.Errorf("invalid parent span ID: %w", err)
		}
		otlpSpan.SetParentSpanID(parentID)
	}

	// Set trace ID
	traceIDStr := extractTraceID(lumigoSpan)
	traceID, err := parseTraceID(traceIDStr)
	if err != nil {
		return fmt.Errorf("invalid trace ID: %w", err)
	}
	otlpSpan.SetTraceID(traceID)

	// Set span name
	otlpSpan.SetName(getSpanName(lumigoSpan))

	// Set span kind
	otlpSpan.SetKind(getSpanKind(lumigoSpan))

	// Set timestamps (Lumigo uses milliseconds)
	otlpSpan.SetStartTimestamp(pcommon.Timestamp(base.Started * 1_000_000))
	otlpSpan.SetEndTimestamp(pcommon.Timestamp(base.Ended * 1_000_000))

	// Set attributes
	setSpanAttributes(otlpSpan.Attributes(), lumigoSpan)

	// Set status and add exception events if there's an error
	setSpanStatus(otlpSpan, lumigoSpan)

	return nil
}

// getSpanName returns the appropriate span name
func getSpanName(span LumigoSpan) string {
	switch s := span.(type) {
	case *FunctionSpan:
		if s.Name != "" {
			return s.Name
		}
		return "AWS Lambda Invocation"
	case *HTTPSpan:
		if s.Info != nil {
			if httpInfo, ok := s.Info["httpInfo"].(map[string]interface{}); ok {
				if request, ok := httpInfo["request"].(map[string]interface{}); ok {
					method := ""
					uri := ""
					if m, ok := request["method"].(string); ok {
						method = m
					}
					if u, ok := request["uri"].(string); ok {
						uri = u
					}
					if method != "" && uri != "" {
						return fmt.Sprintf("%s %s", method, uri)
					}
				}
			}
		}
		return "HTTP Request"
	default:
		return span.GetType()
	}
}

// getSpanKind returns the appropriate span kind
func getSpanKind(span LumigoSpan) ptrace.SpanKind {
	switch span.(type) {
	case *FunctionSpan:
		return ptrace.SpanKindServer
	case *HTTPSpan:
		return ptrace.SpanKindClient
	default:
		return ptrace.SpanKindInternal
	}
}

// setSpanAttributes sets span-level attributes
func setSpanAttributes(attrs pcommon.Map, span LumigoSpan) {
	base := span.GetBaseSpan()

	// Lumigo metadata
	if base.TransactionID != "" {
		attrs.PutStr("lumigo.transaction_id", base.TransactionID)
	}

	if base.LambdaContainerID != "" {
		attrs.PutStr("lumigo.lambda_container_id", base.LambdaContainerID)
	}

	// Type-specific attributes
	switch s := span.(type) {
	case *FunctionSpan:
		setFunctionAttributes(attrs, s)
	case *HTTPSpan:
		setHTTPAttributes(attrs, s)
	}
}

// setSpanStatus sets the span status and adds exception events if there's an error
func setSpanStatus(otlpSpan ptrace.Span, span LumigoSpan) {
	// Check if this is a function span with an error
	if funcSpan, ok := span.(*FunctionSpan); ok && funcSpan.Error != nil {
		// Set error status
		otlpSpan.Status().SetCode(ptrace.StatusCodeError)
		otlpSpan.Status().SetMessage(funcSpan.Error.Message)

		// Add error attributes
		attrs := otlpSpan.Attributes()
		attrs.PutStr("error.type", funcSpan.Error.Type)
		attrs.PutStr("error.message", funcSpan.Error.Message)
		if funcSpan.Error.Stacktrace != "" {
			attrs.PutStr("error.stack", funcSpan.Error.Stacktrace)
		}

		// Add exception event (OpenTelemetry semantic convention)
		event := otlpSpan.Events().AppendEmpty()
		event.SetName("exception")
		event.SetTimestamp(otlpSpan.EndTimestamp()) // Use span end time for exception event
		eventAttrs := event.Attributes()
		eventAttrs.PutStr("exception.type", funcSpan.Error.Type)
		eventAttrs.PutStr("exception.message", funcSpan.Error.Message)
		if funcSpan.Error.Stacktrace != "" {
			eventAttrs.PutStr("exception.stacktrace", funcSpan.Error.Stacktrace)
		}
	} else {
		// No error, set OK status
		otlpSpan.Status().SetCode(ptrace.StatusCodeOk)
	}
}

// setFunctionAttributes sets Lambda function-specific attributes
func setFunctionAttributes(attrs pcommon.Map, span *FunctionSpan) {
	if span.Readiness != "" {
		attrs.PutStr("faas.coldstart", span.Readiness)
	}

	if span.ReturnValue != "" {
		attrs.PutStr("faas.return_value", span.ReturnValue)
	}

	if span.Event != "" {
		attrs.PutStr("faas.event", span.Event)
	}

	if span.ReporterRTT > 0 {
		attrs.PutInt("lumigo.reporter_rtt", int64(span.ReporterRTT))
	}

	// Set faas.invocation_id from the span ID
	attrs.PutStr(semconv.AttributeFaaSInvocationID, span.ID)

	// Add trigger information from Info
	if span.Info != nil {
		if trigger, ok := span.Info["trigger"].([]interface{}); ok && len(trigger) > 0 {
			if triggerMap, ok := trigger[0].(map[string]interface{}); ok {
				if triggeredBy, ok := triggerMap["triggeredBy"].(string); ok {
					attrs.PutStr("faas.trigger", triggeredBy)

					// Add trigger-specific attributes
					if extra, ok := triggerMap["extra"].(map[string]interface{}); ok {
						for key, value := range extra {
							if strVal, ok := value.(string); ok {
								attrs.PutStr(fmt.Sprintf("faas.trigger.%s", key), strVal)
							} else if intVal, ok := value.(float64); ok {
								attrs.PutInt(fmt.Sprintf("faas.trigger.%s", key), int64(intVal))
							}
						}
					}
				}
			}
		}
	}
}

// setHTTPAttributes sets HTTP-specific attributes
func setHTTPAttributes(attrs pcommon.Map, span *HTTPSpan) {
	if span.Info == nil {
		return
	}

	base := span.GetBaseSpan()

	httpInfo, ok := span.Info["httpInfo"].(map[string]interface{})
	if !ok {
		return
	}

	// Request attributes
	if request, ok := httpInfo["request"].(map[string]interface{}); ok {
		if method, ok := request["method"].(string); ok {
			attrs.PutStr(semconv.AttributeHTTPRequestMethod, method)
		}

		if uri, ok := request["uri"].(string); ok {
			attrs.PutStr(semconv.AttributeURLFull, "https://"+uri)
		}

		if headers, ok := request["headers"].(string); ok {
			// Store headers as JSON string
			attrs.PutStr("http.request.headers", headers)
		}
	}

	// Response attributes
	if response, ok := httpInfo["response"].(map[string]interface{}); ok {
		if statusCode, ok := response["statusCode"].(float64); ok {
			attrs.PutInt(semconv.AttributeHTTPResponseStatusCode, int64(statusCode))
		}

		if headers, ok := response["headers"].(string); ok {
			attrs.PutStr("http.response.headers", headers)
		}
	}

	// Host
	if host, ok := httpInfo["host"].(string); ok {
		attrs.PutStr(semconv.AttributeServerAddress, host)
	}

	// NEW: aws.region on span level (for backend compatibility)
	if base.Region != "" {
		attrs.PutStr("aws.region", base.Region)
	}

	// Resource name and message ID
	if resourceName, ok := span.Info["resourceName"].(string); ok {
		attrs.PutStr("aws.resource.name", resourceName)
	}

	if messageID, ok := span.Info["messageId"].(string); ok {
		attrs.PutStr("aws.request.id", messageID)
	}
}

// parseSpanID converts a Lumigo span ID (UUID format) to OTLP span ID (8 bytes)
func parseSpanID(id string) (pcommon.SpanID, error) {
	// Remove hyphens from UUID
	cleanID := strings.ReplaceAll(id, "-", "")

	// Take first 16 characters (8 bytes) from the UUID
	if len(cleanID) >= 16 {
		cleanID = cleanID[:16]
	} else {
		// Pad with zeros if shorter
		cleanID = cleanID + strings.Repeat("0", 16-len(cleanID))
	}

	bytes, err := hex.DecodeString(cleanID)
	if err != nil {
		return pcommon.SpanID{}, err
	}

	var spanID pcommon.SpanID
	copy(spanID[:], bytes)
	return spanID, nil
}

// parseTraceID converts a trace ID to OTLP trace ID (16 bytes)
func parseTraceID(id string) (pcommon.TraceID, error) {
	// Handle AWS X-Ray trace ID format: "1-{epoch}-{unique-id}"
	if strings.HasPrefix(id, "1-") {
		parts := strings.Split(id, "-")
		if len(parts) >= 3 {
			// Use the unique ID part (last 96 bits)
			id = parts[1] + parts[2]
		}
	}

	// Remove hyphens
	cleanID := strings.ReplaceAll(id, "-", "")

	// Ensure we have exactly 32 hex characters (16 bytes)
	if len(cleanID) > 32 {
		cleanID = cleanID[:32]
	} else if len(cleanID) < 32 {
		cleanID = cleanID + strings.Repeat("0", 32-len(cleanID))
	}

	bytes, err := hex.DecodeString(cleanID)
	if err != nil {
		return pcommon.TraceID{}, err
	}

	var traceID pcommon.TraceID
	copy(traceID[:], bytes)
	return traceID, nil
}

// unmarshalLumigoSpans parses JSON into Lumigo spans
func unmarshalLumigoSpans(data []byte, logger *zap.Logger) (LumigoSpanBatch, error) {
	// First, check if it's an array or single object
	var rawData interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	var spans LumigoSpanBatch

	switch v := rawData.(type) {
	case []interface{}:
		// Array of spans
		for i, item := range v {
			itemBytes, err := json.Marshal(item)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal span at index %d: %w", i, err)
			}
			span, err := unmarshalSingleSpan(itemBytes, logger)
			if err != nil {
				// Skip spans that should be filtered out
				if errors.Is(err, ErrSkipSpan) {
					continue
				}
				return nil, fmt.Errorf("failed to unmarshal span at index %d: %w", i, err)
			}
			spans = append(spans, span)
		}
	case map[string]interface{}:
		// Single span
		span, err := unmarshalSingleSpan(data, logger)
		if err != nil {
			// Skip spans that should be filtered out
			if errors.Is(err, ErrSkipSpan) {
				return spans, nil
			}
			return nil, fmt.Errorf("failed to unmarshal single span: %w", err)
		}
		spans = []LumigoSpan{span}
	default:
		return nil, fmt.Errorf("unexpected JSON structure: expected array or object")
	}

	return spans, nil
}

// unmarshalSingleSpan unmarshals a single span based on its type field
func unmarshalSingleSpan(data []byte, logger *zap.Logger) (LumigoSpan, error) {
	// First, peek at the ID to check if it should be skipped
	var idCheck struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &idCheck); err != nil {
		return nil, fmt.Errorf("failed to extract id field: %w", err)
	}

	// Skip "started" spans (spans with IDs ending in "_started")
	if strings.HasSuffix(idCheck.ID, "_started") {
		return nil, ErrSkipSpan
	}

	// Peek at the type field
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return nil, fmt.Errorf("failed to extract type field: %w", err)
	}

	// Unmarshal to the appropriate concrete type
	switch typeCheck.Type {
	case "function":
		var funcSpan FunctionSpan
		if err := json.Unmarshal(data, &funcSpan); err != nil {
			return nil, fmt.Errorf("failed to unmarshal function span: %w", err)
		}
		return &funcSpan, nil
	case "http":
		var httpSpan HTTPSpan
		if err := json.Unmarshal(data, &httpSpan); err != nil {
			return nil, fmt.Errorf("failed to unmarshal http span: %w", err)
		}
		return &httpSpan, nil
	default:
		// Log and skip unsupported span types instead of failing the entire batch
		logger.Error("Skipping span with unsupported type",
			zap.String("span_type", typeCheck.Type),
			zap.String("span_id", idCheck.ID))
		return nil, ErrSkipSpan
	}
}
