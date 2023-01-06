// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package events

import (
	"context"

	"github.com/harness/gitness/events"

	"github.com/rs/zerolog/log"
)

const BranchCreatedEvent events.EventType = "branchcreated"

type BranchCreatedPayload struct {
	RepoID      int64  `json:"repo_id"`
	PrincipalID int64  `json:"principal_id"`
	Ref         string `json:"ref"`
	SHA         string `json:"sha"`
}

func (r *Reporter) BranchCreated(ctx context.Context, payload *BranchCreatedPayload) {
	eventID, err := events.ReporterSendEvent(r.innerReporter, ctx, BranchCreatedEvent, payload)
	if err != nil {
		log.Ctx(ctx).Err(err).Msgf("failed to send branch created event")
		return
	}

	log.Ctx(ctx).Debug().Msgf("reported branch created event with id '%s'", eventID)
}

func (r *Reader) RegisterBranchCreated(fn func(context.Context, *events.Event[*BranchCreatedPayload]) error) error {
	return events.ReaderRegisterEvent(r.innerReader, BranchCreatedEvent, fn)
}