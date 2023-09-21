// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package pullreq

import (
	"context"
	"fmt"
	"time"

	"github.com/harness/gitness/internal/auth"
	"github.com/harness/gitness/types"
	"github.com/harness/gitness/types/enum"

	"github.com/rs/zerolog/log"
)

// CommentDelete deletes a pull request comment.
func (c *Controller) CommentDelete(
	ctx context.Context,
	session *auth.Session,
	repoRef string,
	prNum int64,
	commentID int64,
) error {
	repo, err := c.getRepoCheckAccess(ctx, session, repoRef, enum.PermissionRepoView)
	if err != nil {
		return fmt.Errorf("failed to acquire access to repo: %w", err)
	}

	pr, err := c.pullreqStore.FindByNumber(ctx, repo.ID, prNum)
	if err != nil {
		return fmt.Errorf("failed to find pull request by number: %w", err)
	}

	act, err := c.getCommentCheckEditAccess(ctx, session, pr, commentID)
	if err != nil {
		return fmt.Errorf("failed to get comment: %w", err)
	}

	if act.Deleted != nil {
		return nil
	}

	isBlocking := act.IsBlocking()

	_, err = c.activityStore.UpdateOptLock(ctx, act, func(act *types.PullReqActivity) error {
		now := time.Now().UnixMilli()
		act.Deleted = &now
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to mark comment as deleted: %w", err)
	}

	_, err = c.pullreqStore.UpdateOptLock(ctx, pr, func(pr *types.PullReq) error {
		pr.CommentCount--
		if isBlocking {
			pr.UnresolvedCount--
		}
		return nil
	})
	if err != nil {
		// non-critical error
		log.Ctx(ctx).Err(err).Msgf("failed to decrement pull request comment counters")
	}

	if err = c.sseStreamer.Publish(ctx, repo.ParentID, enum.SSETypePullrequesUpdated, pr); err != nil {
		log.Ctx(ctx).Warn().Msg("failed to publish PR changed event")
	}

	return nil
}
