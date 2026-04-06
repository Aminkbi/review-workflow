package sqlutil

import (
	"context"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"review-workflow/db/migrations"
)

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, span := otel.Tracer("review-workflow/sqlutil").Start(ctx, "postgres.run_migrations")
	defer span.End()

	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		_, migrationSpan := otel.Tracer("review-workflow/sqlutil").Start(ctx, "postgres.apply_migration",
			trace.WithAttributes(attribute.String("db.migration.file", name)))
		sqlBytes, err := migrations.Files.ReadFile(name)
		if err != nil {
			migrationSpan.RecordError(err)
			migrationSpan.SetStatus(codes.Error, err.Error())
			migrationSpan.End()
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			migrationSpan.RecordError(err)
			migrationSpan.SetStatus(codes.Error, err.Error())
			migrationSpan.End()
			return err
		}
		migrationSpan.End()
	}
	return nil
}
