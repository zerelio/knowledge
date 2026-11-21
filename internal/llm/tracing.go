package llm

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("zerelio.llm")

// WithTracing wraps a Provider to add OTel spans to every Embed and Complete call.
func WithTracing(p Provider) Provider {
	if IsNoop(p) {
		return p
	}
	return &tracingProvider{inner: p}
}

type tracingProvider struct {
	inner Provider
}

func (t *tracingProvider) Name() string         { return t.inner.Name() }
func (t *tracingProvider) EmbeddingDim() int    { return t.inner.EmbeddingDim() }

func (t *tracingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, span := tracer.Start(ctx, t.inner.Name()+".embed")
	defer span.End()
	span.SetAttributes(
		attribute.String("llm.provider", t.inner.Name()),
		attribute.Int("llm.texts.count", len(texts)),
	)
	vecs, err := t.inner.Embed(ctx, texts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		if len(vecs) > 0 {
			span.SetAttributes(attribute.Int("llm.embedding.dim", len(vecs[0])))
		}
	}
	return vecs, err
}

func (t *tracingProvider) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	ctx, span := tracer.Start(ctx, t.inner.Name()+".complete")
	defer span.End()
	span.SetAttributes(
		attribute.String("llm.provider", t.inner.Name()),
		attribute.Int("llm.messages.count", len(messages)),
		attribute.Bool("llm.system_prompt", system != ""),
	)
	result, err := t.inner.Complete(ctx, system, messages)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(attribute.Int("llm.output.chars", len(result)))
	}
	return result, err
}
