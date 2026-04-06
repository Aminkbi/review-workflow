package application

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"review-workflow/internal/domain"
)

type SimulatedExecutor struct{}

func (SimulatedExecutor) Execute(ctx context.Context, req domain.Request) error {
	_, span := otel.Tracer("review-workflow/executor").Start(ctx, "executor.simulated",
		trace.WithAttributes(
			attribute.String("workflow.request_id", req.ID),
			attribute.String("workflow.target_resource", req.TargetResource),
			attribute.String("workflow.request_type", req.Type),
		),
	)
	defer span.End()

	if strings.Contains(strings.ToLower(req.TargetResource), "fail") {
		err := fmt.Errorf("simulated provisioning failure for %s", req.TargetResource)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
