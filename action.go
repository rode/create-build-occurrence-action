// Copyright 2021 The Rode Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v35/github"
	collector "github.com/rode/collector-build/proto/v1alpha1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate counterfeiter -o mocks/actions_service.go . actionsService
type actionsService interface {
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error)
}

type createBuildOccurrenceAction struct {
	actions actionsService
	config  *config
	client  collector.BuildCollectorClient
	logger  *zap.Logger
}

func (a *createBuildOccurrenceAction) Run(ctx context.Context) (string, error) {
	owner, repo := getRepoAndOwnerFromSlug(a.config.GitHub.RepoSlug)
	a.logger.Info("Fetching jobs for workflow")
	jobs, _, err := a.actions.ListWorkflowJobs(ctx, owner, repo, a.config.GitHub.RunId, &github.ListWorkflowJobsOptions{})
	if err != nil {
		return "", fmt.Errorf("error listing jobs: %s", err)
	}

	var job *github.WorkflowJob
	for _, j := range jobs.Jobs {
		if j.GetName() == a.config.GitHub.JobId {
			job = j
			break
		}
	}

	if job == nil {
		return "", fmt.Errorf("unable to find job with id %s", a.config.GitHub.JobId)
	}

	repoUri := fmt.Sprintf("%s/%s", a.config.GitHub.ServerUrl, a.config.GitHub.RepoSlug)
	commitUri := fmt.Sprintf("%s/commit/%s", repoUri, a.config.GitHub.CommitId)
	logsUri := fmt.Sprintf("%s/checks/%d/logs", commitUri, job.GetID())
	artifact := buildArtifact(a.config)

	request := &collector.CreateBuildRequest{
		Artifacts:    []*collector.Artifact{artifact},
		BuildStart:   timestamppb.New(job.GetStartedAt().Time),
		BuildEnd:     timestamppb.Now(),
		CommitId:     a.config.GitHub.CommitId,
		CommitUri:    commitUri,
		Creator:      a.config.GitHub.Actor,
		LogsUri:      logsUri,
		ProvenanceId: job.GetHTMLURL(),
		Repository:   repoUri,
	}

	a.logger.Info("Sending request to build collector")
	response, err := a.client.CreateBuild(ctx, request)
	if err != nil {
		return "", fmt.Errorf("error creating build occurrence: %s", err)
	}

	a.logger.Info(fmt.Sprintf("Successfully created build occurrence, id is %s", response.BuildOccurrenceId))

	return response.BuildOccurrenceId, nil
}

func getRepoAndOwnerFromSlug(slug string) (string, string) {
	parts := strings.Split(slug, "/")

	return parts[0], parts[1]
}

func buildArtifact(c *config) *collector.Artifact {
	artifact := &collector.Artifact{
		Id: c.ArtifactId,
	}

	if len(c.ArtifactNames) > 0 {
		artifact.Names = strings.Split(c.ArtifactNames, c.ArtifactNamesDelimiter)
	}

	return artifact
}
