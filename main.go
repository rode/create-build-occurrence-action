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
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v35/github"
	collector "github.com/rode/collector-build/proto/v1alpha1"
	"github.com/sethvargo/go-envconfig"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type buildCollectorConfig struct {
	Host     string `env:"HOST,required"`
	Insecure bool   `env:"INSECURE"`
}

type githubConfig struct {
	Actor     string `env:"ACTOR,required"`
	CommitId  string `env:"SHA,required"`
	JobId     string `env:"JOB,required"`
	RepoSlug  string `env:"REPOSITORY,required"`
	RunId     int64  `env:"RUN_ID,required"`
	ServerUrl string `env:"SERVER_URL,required"`
	Token     string `env:"TOKEN,required"`
}

type config struct {
	AccessToken            string                `env:"ACCESS_TOKEN"`
	ArtifactId             string                `env:"ARTIFACT_ID,required"`
	ArtifactNames          string                `env:"ARTIFACT_NAMES"`
	ArtifactNamesDelimiter string                `env:"ARTIFACT_NAMES_DELIMITER,required"`
	BuildCollector         *buildCollectorConfig `env:",prefix=BUILD_COLLECTOR_"`
	GitHub                 *githubConfig         `env:",prefix=GITHUB_"`
}

type staticCredential struct {
	token                    string
	requireTransportSecurity bool
}

func (s *staticCredential) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + s.token,
	}, nil
}

func (s *staticCredential) RequireTransportSecurity() bool {
	return s.requireTransportSecurity
}

func newBuildCollectorClient(c *config) (*grpc.ClientConn, collector.BuildCollectorClient) {
	dialOptions := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
	}
	if c.BuildCollector.Insecure {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}

	if c.AccessToken != "" {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(&staticCredential{
			token:                    c.AccessToken,
			requireTransportSecurity: !c.BuildCollector.Insecure,
		}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	conn, err := grpc.DialContext(ctx, c.BuildCollector.Host, dialOptions...)
	if err != nil {
		log.Fatalf("Unable to connect to build collector: %v", err)
	}

	return conn, collector.NewBuildCollectorClient(conn)
}

func newLogger() (*zap.Logger, error) {
	return zap.NewDevelopment()
}

func newGitHubClient(c *config) *github.Client {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: c.GitHub.Token,
		},
	)

	return github.NewClient(oauth2.NewClient(context.Background(), tokenSource))
}

func setOutputVariable(name, value string) {
	fmt.Printf("::set-output name=%s::%s", name, value)
}

func fatal(message string) {
	fmt.Println(message)
	os.Exit(1)
}

func main() {
	ctx := context.Background()
	c := &config{}
	if err := envconfig.Process(ctx, c); err != nil {
		fatal(fmt.Sprintf("unable to build config: %s", err))
	}

	logger, err := newLogger()
	if err != nil {
		fatal(fmt.Sprintf("failed to create logger: %s", err))
	}

	conn, client := newBuildCollectorClient(c)
	defer conn.Close()

	githubClient := newGitHubClient(c)

	action := &createBuildOccurrenceAction{
		actions: githubClient.Actions,
		config:  c,
		client:  client,
		logger:  logger,
	}

	occurrenceId, err := action.Run(ctx)
	if err != nil {
		fatal(err.Error())
	}

	setOutputVariable("id", occurrenceId)
}
