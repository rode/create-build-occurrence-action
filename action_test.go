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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v35/github"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	collector "github.com/rode/collector-build/proto/v1alpha1"
	"github.com/rode/create-build-occurrence-action/mocks"
)

var _ = Describe("createBuildOccurrenceAction", func() {
	var (
		ctx            context.Context
		actionsService *mocks.FakeActionsService
		client         *mocks.FakeBuildCollectorClient
		conf           *config
		action         *createBuildOccurrenceAction
	)

	BeforeEach(func() {
		ctx = context.Background()
		conf = &config{
			ArtifactId: fake.URL(),
			BuildCollector: &buildCollectorConfig{
				Host: fake.URL(),
			},
			GitHub: &githubConfig{
				Actor:     fake.Email(),
				CommitId:  fake.LetterN(10),
				JobId:     fake.Word(),
				RepoSlug:  strings.Join([]string{fake.Word(), fake.Word()}, "/"),
				RunId:     fake.Int64(),
				ServerUrl: fake.URL(),
				Token:     fake.LetterN(10),
			},
		}
		client = &mocks.FakeBuildCollectorClient{}
		actionsService = &mocks.FakeActionsService{}

		action = &createBuildOccurrenceAction{
			actions: actionsService,
			client:  client,
			config:  conf,
			logger:  logger,
		}
	})

	Describe("Run", func() {
		var (
			expectedOccurrenceId string
			actualOccurrenceId   string
			actualError          error
		)

		JustBeforeEach(func() {
			actualOccurrenceId, actualError = action.Run(ctx)
		})

		When("successful execution", func() {
			var (
				expectedNumericJobId int64
				expectedJobHtmlUrl   string
				expectedJobStartedAt time.Time
			)

			BeforeEach(func() {
				expectedOccurrenceId = fake.UUID()
				expectedNumericJobId = fake.Int64()
				expectedJobHtmlUrl = fake.URL()
				expectedJobStartedAt = time.Now().UTC().Add(time.Duration(fake.Number(1, 5)) * time.Minute)

				conf.GitHub.ServerUrl = "https://github.com"
				conf.GitHub.RepoSlug = "rode/create-build-occurrence-action"
				conf.GitHub.CommitId = "foobar"

				jobs := &github.Jobs{
					Jobs: []*github.WorkflowJob{
						{
							ID:        github.Int64(expectedNumericJobId),
							HTMLURL:   github.String(expectedJobHtmlUrl),
							StartedAt: &github.Timestamp{Time: expectedJobStartedAt},
							Name:      github.String(conf.GitHub.JobId),
						},
					},
				}

				actionsService.ListWorkflowJobsReturns(jobs, nil, nil)
				client.CreateBuildReturns(&collector.CreateBuildResponse{
					BuildOccurrenceId: expectedOccurrenceId,
				}, nil)
			})

			It("should fetch jobs for the workflow", func() {
				Expect(actionsService.ListWorkflowJobsCallCount()).To(Equal(1))

				_, actualOwner, actualRepo, actualRunId, _ := actionsService.ListWorkflowJobsArgsForCall(0)

				Expect(actualOwner).To(Equal("rode"))
				Expect(actualRepo).To(Equal("create-build-occurrence-action"))
				Expect(actualRunId).To(Equal(conf.GitHub.RunId))
			})

			It("should send a request to the build collector with the correct links", func() {
				expectedLogsUri := fmt.Sprintf("https://github.com/rode/create-build-occurrence-action/commit/foobar/checks/%d/logs", expectedNumericJobId)

				Expect(client.CreateBuildCallCount()).To(Equal(1))
				_, actualRequest, _ := client.CreateBuildArgsForCall(0)

				Expect(actualRequest.Repository).To(Equal("https://github.com/rode/create-build-occurrence-action"))
				Expect(actualRequest.CommitUri).To(Equal("https://github.com/rode/create-build-occurrence-action/commit/foobar"))
				Expect(actualRequest.LogsUri).To(Equal(expectedLogsUri))
				Expect(actualRequest.ProvenanceId).To(Equal(expectedJobHtmlUrl))
			})

			It("should populate the request with build metadata", func() {
				Expect(client.CreateBuildCallCount()).To(Equal(1))
				_, actualRequest, _ := client.CreateBuildArgsForCall(0)

				Expect(actualRequest.Artifacts).To(HaveLen(1))
				Expect(actualRequest.Artifacts[0].Id).To(Equal(conf.ArtifactId))
				Expect(actualRequest.Creator).To(Equal(conf.GitHub.Actor))
				Expect(actualRequest.CommitId).To(Equal(conf.GitHub.CommitId))
				Expect(actualRequest.BuildStart.AsTime()).To(Equal(expectedJobStartedAt))
				Expect(actualRequest.BuildEnd).NotTo(BeNil())
			})

			It("should return the occurrence id", func() {
				Expect(actualOccurrenceId).To(Equal(expectedOccurrenceId))
			})

			It("should not return an error", func() {
				Expect(actualError).NotTo(HaveOccurred())
			})

			When("there are additional artifact names", func() {
				var expectedArtifactNames []string

				BeforeEach(func() {
					expectedArtifactNames = []string{fake.Word(), fake.Word()}

					conf.ArtifactNamesDelimiter = "\n"
					conf.ArtifactNames = strings.Join(expectedArtifactNames, "\n")
				})

				It("should include the names in the request", func() {
					_, actualRequest, _ := client.CreateBuildArgsForCall(0)

					Expect(actualRequest.Artifacts[0].Names).To(ConsistOf(expectedArtifactNames))
				})
			})

			When("there is whitespace in the artifact names", func() {
				BeforeEach(func() {
					artifactNames := []string{fake.Word(), fake.Word()+"\n", ""}

					conf.ArtifactNamesDelimiter = "\n"
					conf.ArtifactNames = strings.Join(artifactNames, "\n")
				})

				It("should strip the whitespace", func() {
					_, actualRequest, _ := client.CreateBuildArgsForCall(0)

					Expect(actualRequest.Artifacts[0].Names).To(HaveLen(2))
				})
			})
		})

		When("an error occurs listing jobs", func() {
			BeforeEach(func() {
				actionsService.ListWorkflowJobsReturns(nil, nil, errors.New(fake.Word()))
			})

			It("should return an error", func() {
				Expect(actualOccurrenceId).To(BeEmpty())
				Expect(actualError).To(HaveOccurred())
				Expect(actualError.Error()).To(ContainSubstring("error listing jobs"))
			})
		})

		When("there are no jobs matching the job id", func() {
			BeforeEach(func() {
				actionsService.ListWorkflowJobsReturns(&github.Jobs{}, nil, nil)
			})

			It("should return an error", func() {
				Expect(actualOccurrenceId).To(BeEmpty())
				Expect(actualError).To(HaveOccurred())
				Expect(actualError.Error()).To(ContainSubstring("unable to find job"))
			})
		})

		When("an error occurs creating the build occurrence", func() {
			BeforeEach(func() {
				jobs := &github.Jobs{
					Jobs: []*github.WorkflowJob{
						{
							Name: github.String(conf.GitHub.JobId),
						},
					},
				}

				actionsService.ListWorkflowJobsReturns(jobs, nil, nil)
				client.CreateBuildReturns(nil, errors.New(fake.Word()))
			})

			It("should return an error", func() {
				Expect(actualOccurrenceId).To(BeEmpty())
				Expect(actualError).To(HaveOccurred())
				Expect(actualError.Error()).To(ContainSubstring("error creating build occurrence"))
			})
		})
	})
})
