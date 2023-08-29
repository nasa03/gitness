// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harness/gitness/internal/store"
	gitness_store "github.com/harness/gitness/store"
	"github.com/harness/gitness/store/database"
	"github.com/harness/gitness/store/database/dbtx"
	"github.com/harness/gitness/types"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

var _ store.TemplateStore = (*templateStore)(nil)

const (
	templateQueryBase = `
		SELECT` + templateColumns + `
		FROM templates`

	templateColumns = `
	template_id,
	template_description,
	template_space_id,
	template_uid,
	template_data,
	template_created,
	template_updated,
	template_version
	`
)

// NewTemplateStore returns a new TemplateStore.
func NewTemplateStore(db *sqlx.DB) *templateStore {
	return &templateStore{
		db: db,
	}
}

type templateStore struct {
	db *sqlx.DB
}

// Find returns a template given a template ID.
func (s *templateStore) Find(ctx context.Context, id int64) (*types.Template, error) {
	const findQueryStmt = templateQueryBase + `
		WHERE template_id = $1`
	db := dbtx.GetAccessor(ctx, s.db)

	dst := new(types.Template)
	if err := db.GetContext(ctx, dst, findQueryStmt, id); err != nil {
		return nil, database.ProcessSQLErrorf(err, "Failed to find template")
	}
	return dst, nil
}

// FindByUID returns a template in a given space with a given UID.
func (s *templateStore) FindByUID(ctx context.Context, spaceID int64, uid string) (*types.Template, error) {
	const findQueryStmt = templateQueryBase + `
		WHERE template_space_id = $1 AND template_uid = $2`
	db := dbtx.GetAccessor(ctx, s.db)

	dst := new(types.Template)
	if err := db.GetContext(ctx, dst, findQueryStmt, spaceID, uid); err != nil {
		return nil, database.ProcessSQLErrorf(err, "Failed to find template")
	}
	return dst, nil
}

// Create creates a template.
func (s *templateStore) Create(ctx context.Context, template *types.Template) error {
	const templateInsertStmt = `
	INSERT INTO templates (
		template_description,
		template_space_id,
		template_uid,
		template_data,
		template_created,
		template_updated,
		template_version
	) VALUES (
		:template_description,
		:template_space_id,
		:template_uid,
		:template_data,
		:template_created,
		:template_updated,
		:template_version
	) RETURNING template_id`
	db := dbtx.GetAccessor(ctx, s.db)

	query, arg, err := db.BindNamed(templateInsertStmt, template)
	if err != nil {
		return database.ProcessSQLErrorf(err, "Failed to bind template object")
	}

	if err = db.QueryRowContext(ctx, query, arg...).Scan(&template.ID); err != nil {
		return database.ProcessSQLErrorf(err, "template query failed")
	}

	return nil
}

func (s *templateStore) Update(ctx context.Context, p *types.Template) error {
	const templateUpdateStmt = `
	UPDATE templates
	SET
		template_description = :template_description,
		template_uid = :template_uid,
		template_data = :template_data,
		template_updated = :template_updated,
		template_version = :template_version
	WHERE template_id = :template_id AND template_version = :template_version - 1`
	updatedAt := time.Now()
	template := *p

	template.Version++
	template.Updated = updatedAt.UnixMilli()

	db := dbtx.GetAccessor(ctx, s.db)

	query, arg, err := db.BindNamed(templateUpdateStmt, template)
	if err != nil {
		return database.ProcessSQLErrorf(err, "Failed to bind template object")
	}

	result, err := db.ExecContext(ctx, query, arg...)
	if err != nil {
		return database.ProcessSQLErrorf(err, "Failed to update template")
	}

	count, err := result.RowsAffected()
	if err != nil {
		return database.ProcessSQLErrorf(err, "Failed to get number of updated rows")
	}

	if count == 0 {
		return gitness_store.ErrVersionConflict
	}

	p.Version = template.Version
	p.Updated = template.Updated
	return nil
}

// UpdateOptLock updates the pipeline using the optimistic locking mechanism.
func (s *templateStore) UpdateOptLock(ctx context.Context,
	template *types.Template,
	mutateFn func(template *types.Template) error,
) (*types.Template, error) {
	for {
		dup := *template

		err := mutateFn(&dup)
		if err != nil {
			return nil, err
		}

		err = s.Update(ctx, &dup)
		if err == nil {
			return &dup, nil
		}
		if !errors.Is(err, gitness_store.ErrVersionConflict) {
			return nil, err
		}

		template, err = s.Find(ctx, template.ID)
		if err != nil {
			return nil, err
		}
	}
}

// List lists all the templates present in a space.
func (s *templateStore) List(ctx context.Context, parentID int64, filter types.ListQueryFilter) ([]*types.Template, error) {
	stmt := database.Builder.
		Select(templateColumns).
		From("templates").
		Where("template_space_id = ?", fmt.Sprint(parentID))

	if filter.Query != "" {
		stmt = stmt.Where("LOWER(template_uid) LIKE ?", fmt.Sprintf("%%%s%%", strings.ToLower(filter.Query)))
	}

	stmt = stmt.Limit(database.Limit(filter.Size))
	stmt = stmt.Offset(database.Offset(filter.Page, filter.Size))

	sql, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert query to sql")
	}

	db := dbtx.GetAccessor(ctx, s.db)

	dst := []*types.Template{}
	if err = db.SelectContext(ctx, &dst, sql, args...); err != nil {
		return nil, database.ProcessSQLErrorf(err, "Failed executing custom list query")
	}

	return dst, nil
}

// Delete deletes a template given a template ID.
func (s *templateStore) Delete(ctx context.Context, id int64) error {
	const templateDeleteStmt = `
		DELETE FROM templates
		WHERE template_id = $1`

	db := dbtx.GetAccessor(ctx, s.db)

	if _, err := db.ExecContext(ctx, templateDeleteStmt, id); err != nil {
		return database.ProcessSQLErrorf(err, "Could not delete template")
	}

	return nil
}

// DeleteByUID deletes a template with a given UID in a space.
func (s *templateStore) DeleteByUID(ctx context.Context, spaceID int64, uid string) error {
	const templateDeleteStmt = `
	DELETE FROM templates
	WHERE template_space_id = $1 AND template_uid = $2`

	db := dbtx.GetAccessor(ctx, s.db)

	if _, err := db.ExecContext(ctx, templateDeleteStmt, spaceID, uid); err != nil {
		return database.ProcessSQLErrorf(err, "Could not delete template")
	}

	return nil
}

// Count of templates in a space.
func (s *templateStore) Count(ctx context.Context, parentID int64, filter types.ListQueryFilter) (int64, error) {
	stmt := database.Builder.
		Select("count(*)").
		From("templates").
		Where("template_space_id = ?", parentID)

	if filter.Query != "" {
		stmt = stmt.Where("LOWER(template_uid) LIKE ?", fmt.Sprintf("%%%s%%", filter.Query))
	}

	sql, args, err := stmt.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "Failed to convert query to sql")
	}

	db := dbtx.GetAccessor(ctx, s.db)

	var count int64
	err = db.QueryRowContext(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, database.ProcessSQLErrorf(err, "Failed executing count query")
	}
	return count, nil
}