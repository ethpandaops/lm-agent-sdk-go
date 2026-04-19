package lmsdk

import (
	"fmt"
	"strings"
)

// typeToJSONSchema converts a Go type string to a JSON Schema type definition.
//
// Type mappings:
//   - "string"           → {"type": "string"}
//   - "int", "int64"     → {"type": "integer"}
//   - "float64", "float" → {"type": "number"}
//   - "bool"             → {"type": "boolean"}
//   - "[]string"         → {"type": "array", "items": {"type": "string"}}
//   - "[]int"            → {"type": "array", "items": {"type": "integer"}}
//   - "[]float64"        → {"type": "array", "items": {"type": "number"}}
//   - "any", "object"    → {"type": "object"}
func typeToJSONSchema(goType string) map[string]any {
	// Handle array types
	if itemType, found := strings.CutPrefix(goType, "[]"); found {
		return map[string]any{
			"type":  "array",
			"items": typeToJSONSchema(itemType),
		}
	}

	// Handle basic types
	switch goType {
	case "string":
		return map[string]any{"type": "string"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return map[string]any{"type": "integer"}
	case "float32", "float64", "float", "number":
		return map[string]any{"type": "number"}
	case "bool", "boolean":
		return map[string]any{"type": "boolean"}
	case "any", "object", "map[string]any":
		return map[string]any{"type": "object"}
	default:
		// Unknown type, default to string
	}

	return map[string]any{"type": "string"}
}

// SchemaBuilder provides a fluent interface for building JSON schemas.
type SchemaBuilder struct {
	properties map[string]any
	required   []string
}

// NewSchemaBuilder creates a new SchemaBuilder.
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		properties: make(map[string]any, 4),
		required:   make([]string, 0, 4),
	}
}

// Property adds a property with the given name and Go type.
// The property is marked as required by default.
func (b *SchemaBuilder) Property(name, goType string) *SchemaBuilder {
	b.properties[name] = typeToJSONSchema(goType)
	b.required = append(b.required, name)

	return b
}

// PropertyWithDescription adds a property with type and description.
func (b *SchemaBuilder) PropertyWithDescription(name, goType, description string) *SchemaBuilder {
	schema := typeToJSONSchema(goType)
	schema["description"] = description
	b.properties[name] = schema
	b.required = append(b.required, name)

	return b
}

// OptionalProperty adds an optional property (not in required list).
func (b *SchemaBuilder) OptionalProperty(name, goType string) *SchemaBuilder {
	b.properties[name] = typeToJSONSchema(goType)

	return b
}

// OptionalPropertyWithDescription adds an optional property with description.
func (b *SchemaBuilder) OptionalPropertyWithDescription(name, goType, description string) *SchemaBuilder {
	schema := typeToJSONSchema(goType)
	schema["description"] = description
	b.properties[name] = schema

	return b
}

// Build returns the complete JSON Schema.
func (b *SchemaBuilder) Build() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": b.properties,
	}

	if len(b.required) > 0 {
		schema["required"] = b.required
	}

	return schema
}

// Validate checks if the input matches the schema requirements.
// Returns an error if required fields are missing.
func Validate(schema, input map[string]any) error {
	required, _ := schema["required"].([]string)
	if required == nil {
		// Try []any (common when unmarshalling from JSON)
		if reqAny, ok := schema["required"].([]any); ok {
			required = make([]string, 0, len(reqAny))
			for _, r := range reqAny {
				if s, isStr := r.(string); isStr {
					required = append(required, s)
				}
			}
		}
	}

	for _, field := range required {
		if _, exists := input[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}
