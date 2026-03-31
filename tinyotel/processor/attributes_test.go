package processor_test

import (
	"testing"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/processor"
)

func spanWithAttrs(attrs map[string]string) model.Span {
	return model.Span{
		TraceID:    "t1",
		SpanID:     "s1",
		Attributes: attrs,
		Resource:   model.Resource{Attributes: map[string]string{}},
	}
}

func TestAttributeInsert(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "insert", Key: "env", Value: "prod"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{})})
	if spans[0].Attributes["env"] != "prod" {
		t.Errorf("insert: env = %q, want prod", spans[0].Attributes["env"])
	}
}

func TestAttributeInsertDoesNotOverwrite(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "insert", Key: "env", Value: "prod"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{"env": "staging"})})
	if spans[0].Attributes["env"] != "staging" {
		t.Errorf("insert should not overwrite existing: env = %q", spans[0].Attributes["env"])
	}
}

func TestAttributeUpdate(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "update", Key: "env", Value: "prod"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{"env": "dev"})})
	if spans[0].Attributes["env"] != "prod" {
		t.Errorf("update: env = %q, want prod", spans[0].Attributes["env"])
	}
}

func TestAttributeUpdateDoesNotInsert(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "update", Key: "env", Value: "prod"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{})})
	if _, ok := spans[0].Attributes["env"]; ok {
		t.Error("update should not insert missing key")
	}
}

func TestAttributeDelete(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "delete", Key: "secret"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{"secret": "token123", "keep": "yes"})})
	if _, ok := spans[0].Attributes["secret"]; ok {
		t.Error("delete: secret should be removed")
	}
	if spans[0].Attributes["keep"] != "yes" {
		t.Error("delete: other keys should be preserved")
	}
}

func TestAttributeRename(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "rename", Key: "http.url", NewKey: "url.full"},
	})
	spans := ap.Process([]model.Span{spanWithAttrs(map[string]string{"http.url": "/api/pods"})})
	if spans[0].Attributes["url.full"] != "/api/pods" {
		t.Errorf("rename: url.full = %q, want /api/pods", spans[0].Attributes["url.full"])
	}
	if _, ok := spans[0].Attributes["http.url"]; ok {
		t.Error("rename: old key should be removed")
	}
}

func TestAttributeRulesAppliedToResourceAttrs(t *testing.T) {
	ap := processor.NewAttributes([]processor.AttributeRule{
		{Action: "insert", Key: "collector.version", Value: "0.1"},
	})
	sp := model.Span{
		TraceID:    "t1",
		SpanID:     "s1",
		Attributes: map[string]string{},
		Resource:   model.Resource{Attributes: map[string]string{}},
	}
	spans := ap.Process([]model.Span{sp})
	if spans[0].Resource.Attributes["collector.version"] != "0.1" {
		t.Errorf("resource attr insert: collector.version = %q", spans[0].Resource.Attributes["collector.version"])
	}
}
