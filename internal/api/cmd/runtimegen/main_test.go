package main

import (
	"reflect"
	"testing"
)

func TestCollectEndpointsOrdersRoutesAndKeepsOnlyJSONSchemas(t *testing.T) {
	request := map[string]any{"$ref": "#/components/schemas/Input"}
	responseSchema := map[string]any{"$ref": "#/components/schemas/Output"}
	parsed := document{Paths: map[string]pathItem{
		"/z/{id}": {
			Post: &operation{
				RequestBody: &body{Required: true, Content: map[string]mediaSchema{
					"application/json": {Schema: request},
				}},
				Responses: map[string]response{
					"201": {Content: map[string]mediaSchema{"application/json": {Schema: responseSchema}}},
					"400": {Reference: "#/components/responses/Problem"},
				},
			},
		},
		"/a": {Get: &operation{Responses: map[string]response{
			"200": {Content: map[string]mediaSchema{"text/plain": {Schema: map[string]any{"type": "string"}}}},
			"204": {},
		}}},
	}}

	got := collectEndpoints(parsed)
	want := []endpoint{
		{Method: "GET", Path: "/a", Responses: map[string]any{}, NoContent: []string{"204"}},
		{Method: "POST", Path: "/z/{id}", RequestRequired: true, Request: request, Responses: map[string]any{"201": responseSchema}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectEndpoints() = %#v, want %#v", got, want)
	}
}

func TestMarshalStableOrdersSchemaKeys(t *testing.T) {
	got, err := marshalStable(map[string]any{"z": 1, "a": 2})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":2,"z":1}` {
		t.Fatalf("marshalStable() = %s", got)
	}
}
